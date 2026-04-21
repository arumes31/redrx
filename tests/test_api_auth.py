import pytest
from app.api import get_user_from_api_key
from app.models import db, URL

def test_get_user_from_api_key_valid(app, test_user):
    with app.test_request_context(headers={'X-API-KEY': 'test-api-key'}):
        user = get_user_from_api_key()
        assert user is not None
        assert user.api_key == 'test-api-key'
        assert user.username == 'testuser'

def test_get_user_from_api_key_missing(app, test_user):
    with app.test_request_context():
        user = get_user_from_api_key()
        assert user is None

def test_get_user_from_api_key_invalid(app, test_user):
    with app.test_request_context(headers={'X-API-KEY': 'wrong-key'}):
        user = get_user_from_api_key()
        assert user is None

def test_api_shorten_auth_success(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://google.com'})
    assert response.status_code == 201

def test_api_shorten_auth_missing(client):
    response = client.post('/api/v1/shorten',
                           json={'long_url': 'https://google.com'})
    assert response.status_code == 401
    assert response.get_json()['error'] == 'Valid API Key required. Access denied.'

def test_api_shorten_auth_invalid(client):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'invalid-key'},
                           json={'long_url': 'https://google.com'})
    assert response.status_code == 401

def test_api_get_info_auth_success(app, client, test_user):
    # Create a URL first
    with app.app_context():
        url = URL(short_code='TESTCODE', long_url='https://google.com', user_id=test_user.id)
        db.session.add(url)
        db.session.commit()

    response = client.get('/api/v1/TESTCODE',
                          headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 200
    assert response.get_json()['short_code'] == 'TESTCODE'

def test_api_get_info_auth_missing(client):
    response = client.get('/api/v1/TESTCODE')
    assert response.status_code == 401

def test_api_get_info_auth_invalid(client):
    response = client.get('/api/v1/TESTCODE',
                          headers={'X-API-KEY': 'invalid-key'})
    assert response.status_code == 401
