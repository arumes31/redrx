import pytest
from app import db
from app.models import User

@pytest.fixture
def auth_config(app):
    app.config['RATELIMIT_LOGIN'] = '1 per minute'
    app.config['RATELIMIT_REGISTER'] = '1 per minute'
    # Force limiter to use memory for testing
    app.config['RATELIMIT_STORAGE_URI'] = 'memory://'
    return app

def test_login_rate_limiting(client, auth_config):
    # First request should succeed (or at least not be 429)
    response = client.post('/login', data={'username': 'test', 'password': 'test'}, follow_redirects=True)
    assert response.status_code != 429

    # Second request should be rate limited
    response = client.post('/login', data={'username': 'test', 'password': 'test'}, follow_redirects=True)
    assert response.status_code == 429

def test_register_rate_limiting(client, auth_config):
    # First request
    response = client.post('/register', data={'username': 'test', 'email': 'test@test.com', 'password': 'test', 'confirm_password': 'test'}, follow_redirects=True)
    assert response.status_code != 429

    # Second request
    response = client.post('/register', data={'username': 'test', 'email': 'test@test.com', 'password': 'test', 'confirm_password': 'test'}, follow_redirects=True)
    assert response.status_code == 429

def test_api_rate_limiting(client, auth_config, test_user):
    # Override API limit
    auth_config.config['RATELIMIT_API'] = '1 per minute'

    # First request
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://google.com'})
    assert response.status_code == 201

    # Second request
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={'long_url': 'https://google.com'})
    assert response.status_code == 429

def test_link_auth_rate_limiting(client, auth_config):
    auth_config.config['RATELIMIT_AUTH'] = '1 per minute'

    # We need a URL with a password to test link-auth
    from app.models import URL
    with auth_config.app_context():
        url = URL(short_code='PASSCODE', long_url='https://google.com', password_hash='pbkdf2:sha256:...')
        db.session.add(url)
        db.session.commit()

    # First request
    response = client.post('/link-auth/PASSCODE', data={'password': 'wrong'}, follow_redirects=True)
    assert response.status_code != 429

    # Second request
    response = client.post('/link-auth/PASSCODE', data={'password': 'wrong'}, follow_redirects=True)
    assert response.status_code == 429
