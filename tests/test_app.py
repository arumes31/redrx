import pytest
import datetime
from app import create_app, db
from app.models import URL
from config import Config

class TestConfig(Config):
    TESTING = True
    SQLALCHEMY_DATABASE_URI = 'sqlite:///:memory:'
    WTF_CSRF_ENABLED = False

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

def test_home_page(client):
    response = client.get('/')
    assert response.status_code == 200
    assert b"Redirx" in response.data

def test_create_short_link(client):
    response = client.post('/', data={
        'long_url': 'https://example.com',
        'custom_code': 'test1'
    }, follow_redirects=True)
    
    assert response.status_code == 200
    assert b"Shortened!" in response.data
    
    with client.application.app_context():
        link = URL.query.filter_by(short_code='TEST1').first()
        assert link is not None
        assert link.long_url == 'https://example.com'

def test_redirect(client):
    # Create link
    client.post('/', data={'long_url': 'https://example.com', 'custom_code': 'redir'})
    
    response = client.get('/REDIR')
    assert response.status_code == 302
    assert response.location == 'https://example.com'

def test_expiry(client):
    # Create expired link
    with client.application.app_context():
        link = URL(
            short_code='EXPIRED', 
            long_url='https://example.com',
            expires_at=datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(days=1)
        )
        db.session.add(link)
        db.session.commit()
    
    response = client.get('/EXPIRED')
    assert response.status_code == 410

def test_404(client):
    response = client.get('/NONEXISTENT')
    assert response.status_code == 404
