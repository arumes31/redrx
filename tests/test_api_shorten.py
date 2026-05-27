import pytest
from unittest.mock import patch
from app.models import db, URL

def test_api_shorten_success(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com'})
    assert response.status_code == 201
    data = response.get_json()
    assert 'short_code' in data
    assert data['long_url'] == 'https://example.com'

def test_api_shorten_custom_conflict(app, client, test_user):
    # Create a URL with a custom code first
    with app.app_context():
        url = URL(short_code='MYCODE', long_url='https://google.com', user_id=test_user.id)
        db.session.add(url)
        db.session.commit()

    # Try to shorten another URL with the same custom code
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'custom_code': 'MYCODE'})

    assert response.status_code == 409
    assert response.get_json()['error'] == 'Custom code already taken'

def test_api_shorten_generated_conflict(app, client, test_user):
    # Create a URL with a specific code first
    with app.app_context():
        url = URL(short_code='COLLID', long_url='https://google.com', user_id=test_user.id)
        db.session.add(url)
        db.session.commit()

    # Mock generate_short_code to return 'COLLID' first (conflict), then 'UNIQUE'
    with patch('app.api.generate_short_code') as mocked_gen:
        mocked_gen.side_effect = ['COLLID', 'UNIQUE']

        response = client.post('/api/v1/shorten',
                               headers={'X-API-KEY': 'test-api-key'},
                               json={'long_url': 'https://example.com'})

        assert response.status_code == 201
        data = response.get_json()
        assert data['short_code'] == 'UNIQUE'
        assert mocked_gen.call_count == 2
