import pytest
import json

def test_shorten_basic(client, test_user):
    response = client.post('/api/v1/shorten',
        headers={'X-API-KEY': 'test-api-key'},
        data=json.dumps({'long_url': 'https://google.com'}),
        content_type='application/json'
    )
    assert response.status_code == 201
    data = response.get_json()
    assert 'short_code' in data
    assert data['long_url'] == 'https://google.com'

def test_shorten_custom_code(client, test_user):
    response = client.post('/api/v1/shorten',
        headers={'X-API-KEY': 'test-api-key'},
        data=json.dumps({
            'long_url': 'https://google.com',
            'custom_code': 'MYCUSTOM'
        }),
        content_type='application/json'
    )
    assert response.status_code == 201
    data = response.get_json()
    assert data['short_code'] == 'MYCUSTOM'

def test_shorten_code_length(client, test_user):
    response = client.post('/api/v1/shorten',
        headers={'X-API-KEY': 'test-api-key'},
        data=json.dumps({
            'long_url': 'https://google.com',
            'code_length': 10
        }),
        content_type='application/json'
    )
    assert response.status_code == 201
    data = response.get_json()
    assert len(data['short_code']) == 10

def test_shorten_rotate_targets(client, test_user):
    targets = ['https://bing.com', 'https://yahoo.com']
    response = client.post('/api/v1/shorten',
        headers={'X-API-KEY': 'test-api-key'},
        data=json.dumps({
            'long_url': 'https://google.com',
            'rotate_targets': targets
        }),
        content_type='application/json'
    )
    assert response.status_code == 201
    data = response.get_json()
    assert data['rotate_targets'] == targets

def test_shorten_password(client, test_user):
    response = client.post('/api/v1/shorten',
        headers={'X-API-KEY': 'test-api-key'},
        data=json.dumps({
            'long_url': 'https://google.com',
            'password': 'secretpassword'
        }),
        content_type='application/json'
    )
    assert response.status_code == 201
    data = response.get_json()
    assert data['password_protected'] is True

def test_shorten_stats_toggle(client, test_user):
    response = client.post('/api/v1/shorten',
        headers={'X-API-KEY': 'test-api-key'},
        data=json.dumps({
            'long_url': 'https://google.com',
            'stats_enabled': False
        }),
        content_type='application/json'
    )
    assert response.status_code == 201
    data = response.get_json()
    assert data['stats_enabled'] is False

def test_shorten_missing_long_url(client, test_user):
    response = client.post('/api/v1/shorten',
        headers={'X-API-KEY': 'test-api-key'},
        data=json.dumps({'custom_code': 'FAIL'}),
        content_type='application/json'
    )
    assert response.status_code == 400
    assert 'Missing long_url' in response.get_json()['error']

def test_shorten_custom_code_collision(client, test_user):
    # First creation
    client.post('/api/v1/shorten',
        headers={'X-API-KEY': 'test-api-key'},
        data=json.dumps({
            'long_url': 'https://google.com',
            'custom_code': 'TAKEN'
        }),
        content_type='application/json'
    )
    # Second attempt
    response = client.post('/api/v1/shorten',
        headers={'X-API-KEY': 'test-api-key'},
        data=json.dumps({
            'long_url': 'https://google.com',
            'custom_code': 'TAKEN'
        }),
        content_type='application/json'
    )
    assert response.status_code == 409
    assert 'already taken' in response.get_json()['error']

def test_shorten_invalid_date(client, test_user):
    response = client.post('/api/v1/shorten',
        headers={'X-API-KEY': 'test-api-key'},
        data=json.dumps({
            'long_url': 'https://google.com',
            'start_at': 'invalid-date'
        }),
        content_type='application/json'
    )
    assert response.status_code == 400
    assert 'Invalid start_at format' in response.get_json()['error']

def test_shorten_rotate_targets_invalid_type(client, test_user):
    response = client.post('/api/v1/shorten',
        headers={'X-API-KEY': 'test-api-key'},
        data=json.dumps({
            'long_url': 'https://google.com',
            'rotate_targets': 'not-a-list'
        }),
        content_type='application/json'
    )
    assert response.status_code == 400
    assert 'must be a list of strings' in response.get_json()['error']
