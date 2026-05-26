import pytest
import time
from flask import url_for

def test_login_rate_limiting(client, app):
    # Set a very low limit for testing
    app.config['RATELIMIT_LOGIN'] = '1 per minute'

    # First request should pass
    response = client.post('/login', data={'username': 'test', 'password': 'test'})
    assert response.status_code != 429

    # Second request should be rate limited
    response = client.post('/login', data={'username': 'test', 'password': 'test'})
    assert response.status_code == 429

def test_register_rate_limiting(client, app):
    app.config['RATELIMIT_REGISTER'] = '1 per minute'

    response = client.post('/register', data={'username': 'test', 'email': 'test@example.com', 'password': 'test', 'confirm_password': 'test'})
    assert response.status_code != 429

    response = client.post('/register', data={'username': 'test', 'email': 'test@example.com', 'password': 'test', 'confirm_password': 'test'})
    assert response.status_code == 429

def test_auth_rate_limiting(client, app):
    app.config['RATELIMIT_AUTH'] = '1 per minute'

    response = client.post('/link-auth/NONEXISTENT', data={'password': 'test'})
    assert response.status_code != 429

    response = client.post('/link-auth/NONEXISTENT', data={'password': 'test'})
    assert response.status_code == 429
