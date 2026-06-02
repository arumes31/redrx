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

def test_api_shorten_multiple_generated_collisions(client, test_user):
    headers = {'X-API-KEY': 'test-api-key'}

    # Pre-populate the DB with several codes
    with client.application.app_context():
        db.session.add(URL(short_code='C1', long_url='https://a.com', user_id=test_user.id))
        db.session.add(URL(short_code='C2', long_url='https://b.com', user_id=test_user.id))
        db.session.add(URL(short_code='C3', long_url='https://c.com', user_id=test_user.id))
        db.session.commit()

    with patch('app.api.generate_short_code') as mock_gen:
        mock_gen.side_effect = ['C1', 'C2', 'C3', 'SUCCESS']

        response = client.post('/api/v1/shorten', headers=headers, json={
            'long_url': 'https://example.com'
        })

        assert response.status_code == 201
        assert response.get_json()['short_code'] == 'SUCCESS'
        assert mock_gen.call_count == 4

def test_api_shorten_generated_collision_respects_code_length(client, test_user):
    headers = {'X-API-KEY': 'test-api-key'}

    with client.application.app_context():
        db.session.add(URL(short_code='COLLIDE', long_url='https://a.com', user_id=test_user.id))
        db.session.commit()

    with patch('app.api.generate_short_code') as mock_gen:
        mock_gen.side_effect = ['COLLIDE', 'UNIQUE']

        response = client.post('/api/v1/shorten', headers=headers, json={
            'long_url': 'https://example.com',
            'code_length': 10
        })

        assert response.status_code == 201
        assert response.get_json()['short_code'] == 'UNIQUE'
        # Verify length 10 was passed to generate_short_code
        mock_gen.assert_called_with(10)
