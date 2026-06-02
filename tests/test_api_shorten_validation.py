import pytest
import datetime
from app.models import db, URL, User

def test_shorten_invalid_json(client, test_user):
    # Flask with application/json content type might just return 400 automatically if invalid JSON is sent
    # Or it might return None for get_json()
    response = client.post('/api/v1/shorten',
                           data="not json",
                           content_type='application/json',
                           headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 400

def test_shorten_missing_long_url(client, test_user):
    response = client.post('/api/v1/shorten',
                           json={},
                           headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 400
    assert 'Missing long_url' in response.get_json()['error']

def test_shorten_invalid_long_url_type(client, test_user):
    response = client.post('/api/v1/shorten',
                           json={'long_url': 123},
                           headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 400
    assert 'must be a string' in response.get_json()['error']

def test_shorten_blocked_url(app, client, test_user):
    with app.app_context():
        import os
        os.environ['BLOCKED_DOMAINS'] = 'malicious.com'

    response = client.post('/api/v1/shorten',
                           json={'long_url': 'http://malicious.com/phish'},
                           headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 403
    assert 'blocked' in response.get_json()['error']

def test_shorten_invalid_custom_code_type(client, test_user):
    response = client.post('/api/v1/shorten',
                           json={'long_url': 'https://google.com', 'custom_code': 123},
                           headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 400
    assert 'must be a string' in response.get_json()['error']

def test_shorten_invalid_custom_code_length(client, test_user):
    # Too short
    response = client.post('/api/v1/shorten',
                           json={'long_url': 'https://google.com', 'custom_code': 'A'},
                           headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 400
    assert 'between 3 and 20' in response.get_json()['error']

def test_shorten_invalid_custom_code_chars(client, test_user):
    response = client.post('/api/v1/shorten',
                           json={'long_url': 'https://google.com', 'custom_code': 'ABC!!!'},
                           headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 400
    assert 'alphanumeric' in response.get_json()['error']

def test_shorten_custom_code_taken(app, client, test_user):
    with app.app_context():
        url = URL(short_code='TAKEN', long_url='https://google.com')
        db.session.add(url)
        db.session.commit()

    response = client.post('/api/v1/shorten',
                           json={'long_url': 'https://google.com', 'custom_code': 'TAKEN'},
                           headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 409
    assert 'already taken' in response.get_json()['error']

def test_shorten_invalid_code_length_type(client, test_user):
    response = client.post('/api/v1/shorten',
                           json={'long_url': 'https://google.com', 'code_length': 'abc'},
                           headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 400
    assert 'must be an integer' in response.get_json()['error']

def test_shorten_invalid_code_length_range(client, test_user):
    response = client.post('/api/v1/shorten',
                           json={'long_url': 'https://google.com', 'code_length': 1},
                           headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 400
    assert 'between 3 and 20' in response.get_json()['error']

def test_shorten_invalid_expiry_hours_type(client, test_user):
    response = client.post('/api/v1/shorten',
                           json={'long_url': 'https://google.com', 'expiry_hours': 'abc'},
                           headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 400
    assert 'must be an integer' in response.get_json()['error']

def test_shorten_invalid_expiry_hours_range(client, test_user):
    response = client.post('/api/v1/shorten',
                           json={'long_url': 'https://google.com', 'expiry_hours': -1},
                           headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 400
    assert 'between 0 and 876,000' in response.get_json()['error']

def test_shorten_invalid_start_at_type(client, test_user):
    response = client.post('/api/v1/shorten',
                           json={'long_url': 'https://google.com', 'start_at': 123},
                           headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 400
    assert 'must be a string' in response.get_json()['error']

def test_shorten_invalid_start_at_format(client, test_user):
    response = client.post('/api/v1/shorten',
                           json={'long_url': 'https://google.com', 'start_at': 'invalid-date'},
                           headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 400
    assert 'Invalid start_at format' in response.get_json()['error']

def test_shorten_invalid_end_at_type(client, test_user):
    response = client.post('/api/v1/shorten',
                           json={'long_url': 'https://google.com', 'end_at': 123},
                           headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 400
    assert 'must be a string' in response.get_json()['error']

def test_shorten_invalid_end_at_format(client, test_user):
    response = client.post('/api/v1/shorten',
                           json={'long_url': 'https://google.com', 'end_at': 'invalid-date'},
                           headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 400
    assert 'Invalid end_at format' in response.get_json()['error']

def test_shorten_invalid_scheduling_window(client, test_user):
    now = datetime.datetime.now(datetime.timezone.utc).isoformat()
    earlier = (datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(hours=1)).isoformat()
    response = client.post('/api/v1/shorten',
                           json={'long_url': 'https://google.com', 'start_at': now, 'end_at': earlier},
                           headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 400
    assert 'end_at must be after start_at' in response.get_json()['error']

def test_shorten_invalid_rotate_targets_type(client, test_user):
    response = client.post('/api/v1/shorten',
                           json={'long_url': 'https://google.com', 'rotate_targets': 'not a list'},
                           headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 400
    assert 'must be a list of strings' in response.get_json()['error']

def test_shorten_invalid_rotate_targets_item_type(client, test_user):
    response = client.post('/api/v1/shorten',
                           json={'long_url': 'https://google.com', 'rotate_targets': ['https://google.com', 123]},
                           headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 400
    assert 'must be a list of strings' in response.get_json()['error']

def test_shorten_too_many_rotate_targets(client, test_user):
    targets = ['https://google.com'] * 51
    response = client.post('/api/v1/shorten',
                           json={'long_url': 'https://google.com', 'rotate_targets': targets},
                           headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 400
    assert 'Maximum 50' in response.get_json()['error']

def test_shorten_blocked_rotate_target(app, client, test_user):
    with app.app_context():
        import os
        os.environ['BLOCKED_DOMAINS'] = 'malicious.com'

    response = client.post('/api/v1/shorten',
                           json={'long_url': 'https://google.com', 'rotate_targets': ['http://malicious.com']},
                           headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 403
    assert 'blocked' in response.get_json()['error']

def test_shorten_success_full(client, test_user):
    now = datetime.datetime.now(datetime.timezone.utc)
    start = now + datetime.timedelta(hours=1)
    end = now + datetime.timedelta(hours=2)

    response = client.post('/api/v1/shorten',
                           json={
                               'long_url': 'https://google.com',
                               'custom_code': 'FULL-TEST',
                               'rotate_targets': ['https://bing.com', 'https://duckduckgo.com'],
                               'password': 'secret-password',
                               'expiry_hours': 24,
                               'preview_mode': False,
                               'stats_enabled': False,
                               'start_at': start.isoformat().replace('+00:00', 'Z'),
                               'end_at': end.isoformat().replace('+00:00', 'Z')
                           },
                           headers={'X-API-KEY': 'test-api-key'})

    assert response.status_code == 201
    data = response.get_json()
    assert data['short_code'] == 'FULL-TEST'
    assert data['rotate_targets'] == ['https://bing.com', 'https://duckduckgo.com']
    assert data['password_protected'] is True
    assert data['preview_mode'] is False
    assert data['stats_enabled'] is False
    assert 'start_at' in data
    assert 'end_at' in data

def test_shorten_expiry_overflow(client, test_user):
    # Try a very large expiry_hours
    client.post('/api/v1/shorten',
                json={'long_url': 'https://google.com', 'expiry_hours': 876000},
                headers={'X-API-KEY': 'test-api-key'})
