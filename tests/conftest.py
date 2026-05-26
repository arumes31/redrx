import pytest
import os
import tempfile
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
            password_hash='pbkdf2:sha256:260000$k98...dummy',
            api_key='test-api-key'
        )
        db.session.add(user)
        db.session.commit()
        # Refresh and access attributes to prevent DetachedInstanceError when accessing them later
        db.session.refresh(user)
        _ = user.id
        _ = user.api_key
        _ = user.username
        db.session.expunge(user)
        return user
