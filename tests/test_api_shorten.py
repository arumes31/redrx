from unittest.mock import patch
from app.models import db, URL

def test_api_shorten_custom_code_collision(client, test_user):
    # Setup: Create a URL with a known short code
    headers = {'X-API-KEY': 'test-api-key'}
    setup_response = client.post('/api/v1/shorten', headers=headers, json={
        'long_url': 'https://example.com/1',
        'custom_code': 'TAKEN'
    })
    assert setup_response.status_code == 201
    assert setup_response.get_json()['short_code'] == 'TAKEN'

    # Action: Try to use the same custom code
    response = client.post('/api/v1/shorten', headers=headers, json={
        'long_url': 'https://example.com/2',
        'custom_code': 'TAKEN'
    })

    # Verification
    assert response.status_code == 409
    assert response.get_json()['error'] == 'Custom code already taken'

def test_api_shorten_generated_code_collision(client, test_user):
    headers = {'X-API-KEY': 'test-api-key'}

    # Pre-populate the DB with a code that will collide
    with client.application.app_context():
        url = URL(short_code='COLLIDE', long_url='https://initial.com', user_id=test_user.id)
        db.session.add(url)
        db.session.commit()

    # Mock generate_short_code to return 'COLLIDE' then 'UNIQUE'
    with patch('app.api.generate_short_code') as mock_gen:
        mock_gen.side_effect = ['COLLIDE', 'UNIQUE']

        response = client.post('/api/v1/shorten', headers=headers, json={
            'long_url': 'https://example.com/unique'
        })

        assert response.status_code == 201
        data = response.get_json()
        assert data['short_code'] == 'UNIQUE'
        assert mock_gen.call_count == 2

def test_api_shorten_with_rotate_targets(client, test_user):
    headers = {'X-API-KEY': 'test-api-key'}
    response = client.post('/api/v1/shorten', headers=headers, json={
        'long_url': 'https://example.com',
        'rotate_targets': [' https://target1.com ', 'https://target2.com']
    })
    assert response.status_code == 201
    data = response.get_json()
    assert data['rotate_targets'] == ['https://target1.com', 'https://target2.com']

def test_api_shorten_with_unsafe_rotate_target(client, test_user):
    headers = {'X-API-KEY': 'test-api-key'}
    # Assuming is_safe_url blocks certain things or we can mock it
    # For now, let's just see if it handles a list correctly.
    # If I want to trigger 403, I'd need to know what's blocked.
    # In config.py, PHISHING_LIST_URLS is set.
    response = client.post('/api/v1/shorten', headers=headers, json={
        'long_url': 'https://example.com',
        'rotate_targets': ['invalid-url']
    })
    # is_safe_url usually checks for http/https and domain
    assert response.status_code == 403
    assert 'blocked or invalid' in response.get_json()['error']
