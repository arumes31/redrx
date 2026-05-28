from app.utils import is_safe_redirect_url
from flask import url_for
from werkzeug.security import generate_password_hash
from app.models import db, User

def test_is_safe_redirect_url():
    # Safe URLs
    assert is_safe_redirect_url('/dashboard') is True
    assert is_safe_redirect_url('/profile?user=1') is True
    assert is_safe_redirect_url('/') is True

    # Unsafe URLs
    assert is_safe_redirect_url('http://evil.com') is False
    assert is_safe_redirect_url('https://evil.com') is False
    assert is_safe_redirect_url('//evil.com') is False
    assert is_safe_redirect_url('ftp://evil.com') is False
    assert is_safe_redirect_url('javascript:alert(1)') is False
    assert is_safe_redirect_url(None) is False
    assert is_safe_redirect_url('') is False
    assert is_safe_redirect_url(123) is False

def test_login_redirect_security(client, app):
    with app.app_context():
        # Ensure we have a SERVER_NAME for url_for to work outside of request if needed,
        # or just use app_context properly.
        app.config['SERVER_NAME'] = 'localhost'
        user = User(username='sectest', email='sec@test.com', password_hash=generate_password_hash('password'))
        db.session.add(user)
        db.session.commit()

        # Test safe redirect
        response = client.post('/login', data={
            'username': 'sectest',
            'password': 'password'
        }, query_string={'next': '/dashboard'}, follow_redirects=False)
        assert response.status_code == 302
        assert response.location.endswith('/dashboard')

        # Test unsafe redirect (should go to index)
        response = client.post('/login', data={
            'username': 'sectest',
            'password': 'password'
        }, query_string={'next': 'http://evil.com'}, follow_redirects=False)
        assert response.status_code == 302
        # Use hardcoded path to avoid url_for issues if it still fails
        assert response.location.endswith('/')

        # Test unsafe protocol-relative redirect (should go to index)
        response = client.post('/login', data={
            'username': 'sectest',
            'password': 'password'
        }, query_string={'next': '//evil.com'}, follow_redirects=False)
        assert response.status_code == 302
        assert response.location.endswith('/')
