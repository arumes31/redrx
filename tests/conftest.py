import pytest
import os
import tempfile
from flask_login import login_user
from werkzeug.security import generate_password_hash
from app import create_app
from app.models import db, User
from config import Config

class TestConfig(Config):
    TESTING = True
    SQLALCHEMY_DATABASE_URI = 'sqlite:///:memory:'
    WTF_CSRF_ENABLED = False
    ENABLE_PHISHING_CHECK = False
    # Use a real path in a temporary directory
    GEOIP_DB_PATH = os.path.join(tempfile.gettempdir(), 'test_geoip.mmdb')

@pytest.fixture
def app():
    app = create_app(TestConfig)

    with app.app_context():
        db.create_all()
        yield app
        db.session.remove()
        db.drop_all()

@pytest.fixture
def client(app):
    return app.test_client()

@pytest.fixture
def runner(app):
    return app.test_cli_runner()

@pytest.fixture
def test_user(app):
    with app.app_context():
        user = User(
            username='testuser',
            email='test@example.com',
            password_hash=generate_password_hash('password'),
            api_key='test-api-key'
        )
        db.session.add(user)
        db.session.commit()
        db.session.refresh(user)
        db.session.expunge(user)
        return user

@pytest.fixture
def other_user(app):
    with app.app_context():
        user = User(
            username='otheruser',
            email='other@example.com',
            password_hash=generate_password_hash('password'),
            api_key='other-api-key'
        )
        db.session.add(user)
        db.session.commit()
        db.session.refresh(user)
        db.session.expunge(user)
        return user

@pytest.fixture
def auth_client(client, test_user, app):
    with app.test_request_context():
        # We need to use the actual client to handle the session cookie
        client.post('/login', data={
            'username': 'testuser',
            'password': 'password'
        }, follow_redirects=True)
    return client
