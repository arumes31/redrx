import pytest
from app.models import URL, db
import datetime

def test_redirect_basic(client, app):
    with app.app_context():
        url = URL(short_code='test', long_url='http://example.com')
        db.session.add(url)
        db.session.commit()

    response = client.get('/test')
    assert response.status_code == 200
    assert b'http://example.com' in response.data

def test_redirect_404(client):
    response = client.get('/nonexistent')
    assert response.status_code == 404

def test_redirect_inactive(client, app):
    with app.app_context():
        # Expired URL
        url = URL(short_code='expired', long_url='http://example.com',
                  expires_at=datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(days=1))
        db.session.add(url)
        db.session.commit()

    response = client.get('/expired')
    assert response.status_code == 410

def test_redirect_password(client, app):
    with app.app_context():
        from werkzeug.security import generate_password_hash
        url = URL(short_code='protected', long_url='http://example.com',
                  password_hash=generate_password_hash('password'))
        db.session.add(url)
        db.session.commit()

    # Should redirect to password auth page
    response = client.get('/protected')
    assert response.status_code == 302
    assert '/link-auth/protected' in response.location

def test_redirect_ios(client, app):
    with app.app_context():
        url = URL(short_code='ios', long_url='http://example.com',
                  ios_target_url='http://apple.com')
        db.session.add(url)
        db.session.commit()

    headers = {'User-Agent': 'Mozilla/5.0 (iPhone; CPU iPhone OS 14_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/14.0 Mobile/15E148 Safari/604.1'}
    response = client.get('/ios', headers=headers)
    assert response.status_code == 200
    assert b'http://apple.com' in response.data

def test_redirect_rotation(client, app):
    with app.app_context():
        url = URL(short_code='rotate', long_url='http://example.com',
                  rotate_targets=['http://alt1.com', 'http://alt2.com'])
        db.session.add(url)
        db.session.commit()

    response = client.get('/rotate')
    assert response.status_code == 200
    assert any(b in response.data for b in [b'http://alt1.com', b'http://alt2.com'])
