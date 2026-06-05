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

def test_api_shorten_valid_dates(client, test_user):
    headers = {'X-API-KEY': 'test-api-key'}
    start_at = "2025-01-01T00:00:00Z"
    end_at = "2025-12-31T23:59:59+00:00"

    response = client.post('/api/v1/shorten', headers=headers, json={
        'long_url': 'https://example.com',
        'start_at': start_at,
        'end_at': end_at
    })

    assert response.status_code == 201
    data = response.get_json()
    # fromisoformat(start_at.replace('Z', '+00:00')).isoformat() -> '2025-01-01T00:00:00+00:00'
    assert data['start_at'] == '2025-01-01T00:00:00+00:00'
    assert data['end_at'] == '2025-12-31T23:59:59+00:00'

def test_api_shorten_invalid_start_at(client, test_user):
    headers = {'X-API-KEY': 'test-api-key'}
    response = client.post('/api/v1/shorten', headers=headers, json={
        'long_url': 'https://example.com',
        'start_at': 'invalid-date'
    })

    assert response.status_code == 400
    assert response.get_json()['error'] == 'Invalid start_at format. Use ISO 8601'

def test_api_shorten_invalid_end_at(client, test_user):
    headers = {'X-API-KEY': 'test-api-key'}
    response = client.post('/api/v1/shorten', headers=headers, json={
        'long_url': 'https://example.com',
        'end_at': 'invalid-date'
    })

    assert response.status_code == 400
    assert response.get_json()['error'] == 'Invalid end_at format. Use ISO 8601'

def test_api_shorten_invalid_window(client, test_user):
    headers = {'X-API-KEY': 'test-api-key'}
    response = client.post('/api/v1/shorten', headers=headers, json={
        'long_url': 'https://example.com',
        'start_at': '2025-12-31T23:59:59Z',
        'end_at': '2025-01-01T00:00:00Z'
    })

    assert response.status_code == 400
    assert response.get_json()['error'] == 'Invalid scheduling window: end_at must be after start_at'

def test_api_shorten_validate_password(client, test_user):
    headers = {'X-API-KEY': 'test-api-key'}
    response = client.post('/api/v1/shorten', headers=headers, json={
        'long_url': 'https://example.com',
        'password': 12345
    })
    assert response.status_code == 400
    assert response.get_json()['error'] == 'password must be a string'

def test_api_shorten_validate_preview_mode(client, test_user):
    headers = {'X-API-KEY': 'test-api-key'}
    response = client.post('/api/v1/shorten', headers=headers, json={
        'long_url': 'https://example.com',
        'preview_mode': 'invalid_bool'
    })
    assert response.status_code == 400
    assert response.get_json()['error'] == 'preview_mode must be a boolean'

    response = client.post('/api/v1/shorten', headers=headers, json={
        'long_url': 'https://example.com',
        'preview_mode': 'false'
    })
    assert response.status_code == 201
    assert response.get_json()['preview_mode'] is False

def test_api_shorten_validate_stats_enabled(client, test_user):
    headers = {'X-API-KEY': 'test-api-key'}
    response = client.post('/api/v1/shorten', headers=headers, json={
        'long_url': 'https://example.com',
        'stats_enabled': 'invalid_bool'
    })
    assert response.status_code == 400
    assert response.get_json()['error'] == 'stats_enabled must be a boolean'

    response = client.post('/api/v1/shorten', headers=headers, json={
        'long_url': 'https://example.com',
        'stats_enabled': '1'
    })
    assert response.status_code == 201
    assert response.get_json()['stats_enabled'] is True

def test_api_shorten_device_targeting(client, test_user):
    headers = {'X-API-KEY': 'test-api-key'}
    response = client.post('/api/v1/shorten', headers=headers, json={
        'long_url': 'https://example.com',
        'ios_target_url': 'https://apps.apple.com/app/id123',
        'android_target_url': 'https://play.google.com/store/apps/details?id=com.example'
    })
    assert response.status_code == 201
    data = response.get_json()
    assert data['ios_target_url'] == 'https://apps.apple.com/app/id123'
    assert data['android_target_url'] == 'https://play.google.com/store/apps/details?id=com.example'

    # Verify via GET endpoint
    get_response = client.get(f"/api/v1/{data['short_code']}", headers=headers)
    assert get_response.status_code == 200
    get_data = get_response.get_json()
    assert get_data['ios_target_url'] == 'https://apps.apple.com/app/id123'
    assert get_data['android_target_url'] == 'https://play.google.com/store/apps/details?id=com.example'

def test_api_shorten_device_targeting_invalid(client, test_user):
    headers = {'X-API-KEY': 'test-api-key'}
    # Non-string value
    response = client.post('/api/v1/shorten', headers=headers, json={
        'long_url': 'https://example.com',
        'ios_target_url': 12345
    })
    assert response.status_code == 400
    assert response.get_json()['error'] == 'ios_target_url must be a string'

    # Blocked/invalid URL
    response = client.post('/api/v1/shorten', headers=headers, json={
        'long_url': 'https://example.com',
        'android_target_url': 'invalid-url-schema'
    })
    assert response.status_code == 403
    assert 'blocked or invalid' in response.get_json()['error']
