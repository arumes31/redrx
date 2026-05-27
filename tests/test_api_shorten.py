import pytest
from app.models import db, URL
import datetime

def test_api_shorten_minimal(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com'})
    assert response.status_code == 201
    data = response.get_json()
    assert 'short_code' in data
    assert data['long_url'] == 'https://example.com'

def test_api_shorten_custom_code(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'custom_code': 'MYCODE'})
    assert response.status_code == 201
    assert response.get_json()['short_code'] == 'MYCODE'

def test_api_shorten_custom_code_collision(client, test_user, app):
    with app.app_context():
        url = URL(short_code='TAKEN', long_url='https://already.exists')
        db.session.add(url)
        db.session.commit()

    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'custom_code': 'TAKEN'})
    assert response.status_code == 409
    assert response.get_json()['error'] == 'Custom code already taken'

def test_api_shorten_unsafe_url(client, test_user):
    # 'blocked.com' is often a default blocked domain or we can mock it if needed.
    # Looking at app/utils.py, it checks BLOCKED_DOMAINS env var.
    import os
    os.environ['BLOCKED_DOMAINS'] = 'malicious.com'

    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://malicious.com'})
    assert response.status_code == 403
    assert response.get_json()['error'] == 'Destination URL is blocked'

def test_api_shorten_invalid_dates(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'start_at': 'invalid-date'})
    assert response.status_code == 400
    assert 'Invalid start_at format' in response.get_json()['error']

def test_api_shorten_rotate_targets_valid(client, test_user):
    targets = ['https://site1.com', 'https://site2.com']
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'rotate_targets': targets})
    assert response.status_code == 201
    assert response.get_json()['rotate_targets'] == targets

def test_api_shorten_rotate_targets_invalid_type(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'rotate_targets': 'not-a-list'})
    assert response.status_code == 400
    assert 'rotate_targets must be a list of strings' in response.get_json()['error']

def test_api_shorten_rotate_targets_too_many(client, test_user):
    targets = ['https://site.com'] * 51
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'rotate_targets': targets})
    assert response.status_code == 400
    assert 'Maximum 50 rotate targets allowed' in response.get_json()['error']

def test_api_shorten_rotate_targets_unsafe(client, test_user):
    import os
    os.environ['BLOCKED_DOMAINS'] = 'bad.com'
    targets = ['https://safe.com', 'https://bad.com']
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'rotate_targets': targets})
    assert response.status_code == 403
    assert 'One or more rotate target URLs are blocked' in response.get_json()['error']

def test_api_shorten_expiry(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'expiry_hours': 24})
    assert response.status_code == 201
    data = response.get_json()
    assert data['expires_at'] is not None
