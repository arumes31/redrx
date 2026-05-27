import pytest
from flask import url_for
from app.models import User, db
from werkzeug.security import generate_password_hash

def test_login_open_redirect_fix(client, app):
    # Create a test user
    with app.app_context():
        user = User(username='security_test', email='security@test.com', password_hash=generate_password_hash('password123'))
        db.session.add(user)
        db.session.commit()

    # Attempt to login with an external 'next' parameter
    malicious_url = 'https://malicious.com'
    response = client.post('/login', data={
        'username': 'security_test',
        'password': 'password123'
    }, query_string={'next': malicious_url}, follow_redirects=False)

    # After fix, it should NOT redirect to malicious_url, but to index (/)
    assert response.status_code == 302
    assert response.location == 'http://localhost/' or response.location == '/'

def test_login_protocol_relative_redirect_fix(client, app):
    # Create a test user
    with app.app_context():
        # User already created in previous test if it persisted, but conftest uses in-memory db with fresh start per app fixture
        user = User(username='security_test_2', email='security2@test.com', password_hash=generate_password_hash('password123'))
        db.session.add(user)
        db.session.commit()

    # Attempt to login with a protocol-relative 'next' parameter
    malicious_url = '//malicious.com'
    response = client.post('/login', data={
        'username': 'security_test_2',
        'password': 'password123'
    }, query_string={'next': malicious_url}, follow_redirects=False)

    # After fix, it should NOT redirect to malicious_url, but to index (/)
    assert response.status_code == 302
    assert response.location == 'http://localhost/' or response.location == '/'

def test_login_safe_redirect(client, app):
    # Create a test user
    with app.app_context():
        user = User(username='security_test_safe', email='security_safe@test.com', password_hash=generate_password_hash('password123'))
        db.session.add(user)
        db.session.commit()

    # Attempt to login with a safe 'next' parameter
    safe_url = '/dashboard'
    response = client.post('/login', data={
        'username': 'security_test_safe',
        'password': 'password123'
    }, query_string={'next': safe_url}, follow_redirects=False)

    assert response.status_code == 302
    assert response.location.endswith(safe_url) or response.location == 'http://localhost' + safe_url
