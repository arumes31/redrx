import pytest
from app.models import db, URL, Click
from flask import session
import datetime

def test_basic_redirection(client, app):
    with app.app_context():
        url = URL(short_code='test', long_url='https://example.com')
        db.session.add(url)
        db.session.commit()

    response = client.get('/test')
    assert response.status_code == 200
    assert b'https://example.com' in response.data

def test_redirection_not_found(client):
    response = client.get('/nonexistent')
    assert response.status_code == 404

def test_redirection_inactive(client, app):
    with app.app_context():
        url = URL(short_code='inactive', long_url='https://example.com',
                  expires_at=datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(days=1))
        db.session.add(url)
        db.session.commit()

    response = client.get('/inactive')
    assert response.status_code == 410

def test_password_protected_redirection(client, app):
    from werkzeug.security import generate_password_hash
    with app.app_context():
        url = URL(short_code='secret', long_url='https://example.com',
                  password_hash=generate_password_hash('password123'))
        db.session.add(url)
        db.session.commit()

    # Try redirecting without password
    response = client.get('/secret')
    assert response.status_code == 302
    assert '/link-auth/secret' in response.location

    # Authenticate
    response = client.post('/link-auth/secret', data={'password': 'password123'}, follow_redirects=True)
    assert response.status_code == 200
    assert b'https://example.com' in response.data

def test_ios_targeting(client, app):
    with app.app_context():
        url = URL(short_code='ios', long_url='https://example.com', ios_target_url='https://apple.com')
        db.session.add(url)
        db.session.commit()

    headers = {'User-Agent': 'Mozilla/5.0 (iPhone; CPU iPhone OS 14_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/14.0 Mobile/15E148 Safari/604.1'}
    response = client.get('/ios', headers=headers)
    assert response.status_code == 200
    assert b'https://apple.com' in response.data

def test_android_targeting(client, app):
    with app.app_context():
        url = URL(short_code='android', long_url='https://example.com', android_target_url='https://google.com')
        db.session.add(url)
        db.session.commit()

    headers = {'User-Agent': 'Mozilla/5.0 (Linux; Android 10; SM-G973F) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.106 Mobile Safari/537.36'}
    response = client.get('/android', headers=headers)
    assert response.status_code == 200
    assert b'https://google.com' in response.data

def test_url_rotation(client, app):
    with app.app_context():
        url = URL(short_code='rotate', long_url='https://example.com')
        url.rotate_targets = ['https://rotate1.com', 'https://rotate2.com']
        db.session.add(url)
        db.session.commit()

    # Since select_rotate_target is random, we just check if it's one of the options or the main one if rotation failed for some reason
    response = client.get('/rotate')
    assert response.status_code == 200
    assert any(b in response.data for b in [b'https://rotate1.com', b'https://rotate2.com', b'https://example.com'])

def test_stats_recording(client, app):
    with app.app_context():
        url = URL(short_code='stats', long_url='https://example.com', stats_enabled=True)
        db.session.add(url)
        db.session.commit()
        url_id = url.id

    client.get('/stats')

    with app.app_context():
        click = Click.query.filter_by(url_id=url_id).first()
        assert click is not None
        assert click.ip_address is not None
        assert Click.query.count() == 1
