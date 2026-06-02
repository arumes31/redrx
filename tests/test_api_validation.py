from unittest.mock import patch
import datetime

def test_shorten_invalid_payload_not_dict(client, test_user):
    # Sending a JSON list instead of a JSON object
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json=["not", "a", "dict"])
    assert response.status_code == 400
    assert response.get_json()['error'] == 'Request payload must be a JSON object'

def test_shorten_missing_long_url(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'custom_code': 'TEST'})
    assert response.status_code == 400
    assert response.get_json()['error'] == 'Missing long_url'

def test_shorten_long_url_not_string(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 123})
    assert response.status_code == 400
    assert response.get_json()['error'] == 'long_url must be a string'

def test_shorten_blocked_url(client, test_user):
    with patch('app.api.is_safe_url', return_value=False):
        response = client.post('/api/v1/shorten',
                               headers={'X-API-KEY': 'test-api-key'},
                               json={'long_url': 'https://blocked.com'})
        assert response.status_code == 403
        assert response.get_json()['error'] == 'Destination URL is blocked'

def test_shorten_custom_code_not_string(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'custom_code': 123})
    assert response.status_code == 400
    assert response.get_json()['error'] == 'custom_code must be a string'

def test_shorten_custom_code_invalid_length(client, test_user):
    # Too short
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'custom_code': 'AB'})
    assert response.status_code == 400

    # Too long
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'custom_code': 'A' * 21})
    assert response.status_code == 400

def test_shorten_custom_code_invalid_chars(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'custom_code': 'INVALID CHARS!'})
    assert response.status_code == 400
    assert 'custom_code must contain only alphanumeric characters' in response.get_json()['error']

def test_shorten_invalid_code_length_type(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'code_length': 'not-an-int'})
    assert response.status_code == 400
    assert response.get_json()['error'] == 'code_length must be an integer'

def test_shorten_code_length_out_of_range(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'code_length': 2})
    assert response.status_code == 400

    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'code_length': 21})
    assert response.status_code == 400

def test_shorten_invalid_expiry_hours_type(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'expiry_hours': 'not-an-int'})
    assert response.status_code == 400
    assert response.get_json()['error'] == 'expiry_hours must be an integer'

def test_shorten_expiry_hours_out_of_range(client, test_user):
    # Min range
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'expiry_hours': -1})
    assert response.status_code == 400

    # Max range
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'expiry_hours': 1000000})
    assert response.status_code == 400

def test_shorten_invalid_start_at_type(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'start_at': 123})
    assert response.status_code == 400
    assert response.get_json()['error'] == 'start_at must be a string (ISO 8601)'

def test_shorten_invalid_end_at_type(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'end_at': 123})
    assert response.status_code == 400
    assert response.get_json()['error'] == 'end_at must be a string (ISO 8601)'

def test_shorten_invalid_start_at_format(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'start_at': 'not-a-date'})
    assert response.status_code == 400
    assert 'Invalid start_at format' in response.get_json()['error']

def test_shorten_invalid_end_at_format(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'end_at': 'not-a-date'})
    assert response.status_code == 400
    assert 'Invalid end_at format' in response.get_json()['error']

def test_shorten_invalid_scheduling_window(client, test_user):
    start = datetime.datetime.now(datetime.timezone.utc).isoformat()
    end = (datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(hours=1)).isoformat()
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'start_at': start, 'end_at': end})
    assert response.status_code == 400
    assert response.get_json()['error'] == 'Invalid scheduling window: end_at must be after start_at'

def test_shorten_with_password(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'password': 'securepassword'})
    assert response.status_code == 201
    assert response.get_json()['password_protected'] is True

def test_shorten_invalid_rotate_targets_type(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'rotate_targets': 'not-a-list'})
    assert response.status_code == 400
    assert response.get_json()['error'] == 'rotate_targets must be a list of strings'

def test_shorten_rotate_targets_not_strings(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'rotate_targets': ['https://a.com', 123]})
    assert response.status_code == 400
    assert response.get_json()['error'] == 'rotate_targets must be a list of strings'

def test_shorten_rotate_targets_too_many(client, test_user):
    targets = ['https://example.com'] * 51
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'rotate_targets': targets})
    assert response.status_code == 400
    assert response.get_json()['error'] == 'Maximum 50 rotate targets allowed'

def test_shorten_rotate_targets_blocked(client, test_user):
    with patch('app.api.is_safe_url') as mock_safe:
        # First call (main long_url) is safe, second (rotate target) is not
        mock_safe.side_effect = [True, False]
        response = client.post('/api/v1/shorten',
                               headers={'X-API-KEY': 'test-api-key'},
                               json={'long_url': 'https://safe.com', 'rotate_targets': ['https://blocked.com']})
        assert response.status_code == 403
        assert 'rotate target URLs are blocked' in response.get_json()['error']

def test_shorten_expiry_hours_overflow(client, test_user):
    with patch('datetime.timedelta') as mock_delta:
        mock_delta.side_effect = OverflowError("overflow")
        response = client.post('/api/v1/shorten',
                               headers={'X-API-KEY': 'test-api-key'},
                               json={'long_url': 'https://example.com', 'expiry_hours': 24})
        assert response.status_code == 400
        assert 'expiry_hours results in a date that is out of range' in response.get_json()['error']

def test_shorten_valid_rotate_targets(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://example.com', 'rotate_targets': ['https://a.com', 'https://b.com']})
    assert response.status_code == 201
    assert response.get_json()['rotate_targets'] == ['https://a.com', 'https://b.com']
