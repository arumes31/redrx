import pytest
from flask import url_for
from app.models import User, db
from werkzeug.security import generate_password_hash

@pytest.fixture
def login_user_setup(app):
    with app.app_context():
        user = User(
            username='redirect_test_user',
            email='redirect@example.com',
            password_hash=generate_password_hash('password123'),
            api_key='redirect-api-key'
        )
        db.session.add(user)
        db.session.commit()
        return user

def test_login_redirect_safe(client, app, login_user_setup):
    response = client.post('/login?next=/dashboard', data={
        'username': 'redirect_test_user',
        'password': 'password123'
    }, follow_redirects=False)

    assert response.status_code == 302
    assert response.location.endswith('/dashboard')

def test_login_redirect_unsafe(client, app, login_user_setup):
    response = client.post('/login?next=http://attacker.com', data={
        'username': 'redirect_test_user',
        'password': 'password123'
    }, follow_redirects=False)

    assert response.status_code == 302
    assert 'attacker.com' not in response.location
    assert response.location.endswith('/')
