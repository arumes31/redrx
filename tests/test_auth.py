import pytest
from app import create_app, db
from app.models import User
from werkzeug.security import generate_password_hash
from config import Config

class TestConfig(Config):
    TESTING = True
    SQLALCHEMY_DATABASE_URI = 'sqlite:///:memory:'
    WTF_CSRF_ENABLED = False
    BASE_DOMAIN = 'short.example.com'

@pytest.fixture
def app():
    app = create_app(TestConfig)
    with app.app_context():
        db.create_all()
        user = User(
            username='testuser',
            email='test@example.com',
            password_hash=generate_password_hash('password123')
        )
        db.session.add(user)
        db.session.commit()
    yield app

@pytest.fixture
def client(app):
    return app.test_client()

def test_safe_login_redirect_vulnerability_fixed(client):
    # This test should now redirect to index because malicious.com is NOT safe
    response = client.post('/login?next=http://malicious.com', data={
        'username': 'testuser',
        'password': 'password123'
    }, follow_redirects=False)

    assert response.status_code == 302
    # Should redirect to index (/)
    assert response.location == '/' or response.location == 'http://localhost/'

def test_safe_login_redirect_allowed(client):
    response = client.post('/login?next=/dashboard', data={
        'username': 'testuser',
        'password': 'password123'
    }, follow_redirects=False)

    assert response.status_code == 302
    assert response.location == '/dashboard' or response.location == 'http://localhost/dashboard'
