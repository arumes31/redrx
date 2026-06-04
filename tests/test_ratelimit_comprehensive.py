import pytest
from app import create_app, db, limiter
from config import Config

class TestConfig(Config):
    TESTING = True
    SQLALCHEMY_DATABASE_URI = 'sqlite:///:memory:'
    WTF_CSRF_ENABLED = False
    RATELIMIT_ENABLED = True
    RATELIMIT_LOGIN = "1 per minute"
    RATELIMIT_REGISTER = "1 per minute"
    RATELIMIT_AUTH = "1 per minute"
    RATELIMIT_API = "1 per minute"
    RATELIMIT_CREATE = "1 per minute"

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

def test_login_rate_limit(client):
    client.post('/login', data={'username': 'test', 'password': 'test'})
    response = client.post('/login', data={'username': 'test', 'password': 'test'})
    assert response.status_code == 429

def test_register_rate_limit(client):
    client.post('/register', data={'username': 'test', 'email': 'test@example.com', 'password': 'password', 'confirm': 'password'})
    response = client.post('/register', data={'username': 'test', 'email': 'test@example.com', 'password': 'password', 'confirm': 'password'})
    assert response.status_code == 429

def test_api_shorten_rate_limit(client, app):
    from app.models import User
    from werkzeug.security import generate_password_hash
    with app.app_context():
        user = User(username='test', email='test@test.com', api_key='test-key', password_hash=generate_password_hash('password'))
        db.session.add(user)
        db.session.commit()

    headers = {'X-API-KEY': 'test-key'}
    client.post('/api/v1/shorten', json={'long_url': 'http://example.com'}, headers=headers)
    response = client.post('/api/v1/shorten', json={'long_url': 'http://example.com'}, headers=headers)
    assert response.status_code == 429

def test_api_info_rate_limit(client, app):
    from app.models import User, URL
    from werkzeug.security import generate_password_hash
    with app.app_context():
        user = User(username='test', email='test@test.com', api_key='test-key', password_hash=generate_password_hash('password'))
        db.session.add(user)
        url = URL(short_code='TEST', long_url='http://example.com', user_id=user.id)
        db.session.add(url)
        db.session.commit()

    headers = {'X-API-KEY': 'test-key'}
    client.get('/api/v1/TEST', headers=headers)
    response = client.get('/api/v1/TEST', headers=headers)
    assert response.status_code == 429

def test_logout_rate_limit(client):
    client.get('/logout')
    response = client.get('/logout')
    assert response.status_code == 429

def test_edit_url_rate_limit(client, app):
    from app.models import User, URL
    from werkzeug.security import generate_password_hash
    with app.app_context():
        user = User(id=1, username='test', email='test@test.com', password_hash=generate_password_hash('password'))
        db.session.add(user)
        url = URL(short_code='TEST', long_url='http://example.com', user_id=user.id)
        db.session.add(url)
        db.session.commit()

    with client.session_transaction() as sess:
        sess['_user_id'] = '1'
        sess['_fresh'] = True

    client.get('/edit/TEST')
    response = client.get('/edit/TEST')
    assert response.status_code == 429

def test_delete_url_rate_limit(client, app):
    from app.models import User, URL
    from werkzeug.security import generate_password_hash
    with app.app_context():
        user = User(id=1, username='test', email='test@test.com', password_hash=generate_password_hash('password'))
        db.session.add(user)
        url = URL(short_code='TEST', long_url='http://example.com', user_id=user.id)
        db.session.add(url)
        db.session.commit()

    with client.session_transaction() as sess:
        sess['_user_id'] = '1'
        sess['_fresh'] = True

    client.post('/delete/TEST')
    response = client.post('/delete/TEST')
    assert response.status_code == 429
