import pytest
import time
from app import create_app, db, limiter
from app.models import User
from config import Config

class TestConfig(Config):
    TESTING = True
    DEBUG = True
    SQLALCHEMY_DATABASE_URI = 'sqlite:///:memory:'
    WTF_CSRF_ENABLED = False
    # Set tight limits for testing
    RATELIMIT_LOGIN = "1 per minute"
    RATELIMIT_ENABLED = True

@pytest.fixture(autouse=True)
def clear_limiter(app):
    with app.app_context():
        limiter.reset()

@pytest.fixture
def app():
    app = create_app(TestConfig)
    with app.app_context():
        db.create_all()
    yield app

@pytest.fixture
def client(app):
    return app.test_client()

def test_login_rate_limiting(client):
    # First attempt
    response = client.post('/login', data={
        'username': 'nonexistent',
        'password': 'wrongpassword'
    })
    # Should not be rate limited yet (either 200 or 302/401 depending on logic)
    assert response.status_code != 429

    # Second attempt (should be blocked by "1 per minute")
    response = client.post('/login', data={
        'username': 'nonexistent',
        'password': 'wrongpassword'
    })
    assert response.status_code == 429
