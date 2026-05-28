import pytest
from app.models import db, URL, Click

def test_redirect_basic(client, app):
    with app.app_context():
        url = URL(short_code='TEST1', long_url='http://example.com')
        db.session.add(url)
        db.session.commit()

    response = client.get('/TEST1')
    assert response.status_code == 200
    assert b'http://example.com' in response.data

def test_redirect_404(client):
    response = client.get('/NONEXISTENT')
    assert response.status_code == 404

def test_redirect_stats(client, app):
    with app.app_context():
        url = URL(short_code='STATS', long_url='http://example.com', stats_enabled=True)
        db.session.add(url)
        db.session.commit()
        url_id = url.id

    response = client.get('/STATS')
    assert response.status_code == 200

    with app.app_context():
        url = db.session.get(URL, url_id)
        assert url.clicks_count == 1
        click = Click.query.filter_by(url_id=url_id).first()
        assert click is not None
        assert click.referrer == 'Direct'

def test_redirect_device_targeting_ios(client, app):
    with app.app_context():
        url = URL(
            short_code='DEVICE',
            long_url='http://example.com',
            ios_target_url='http://ios.example.com'
        )
        db.session.add(url)
        db.session.commit()

    headers = {'User-Agent': 'Mozilla/5.0 (iPhone; CPU iPhone OS 14_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/14.0 Mobile/15E148 Safari/604.1'}
    response = client.get('/DEVICE', headers=headers)
    assert b'http://ios.example.com' in response.data

def test_redirect_device_targeting_android(client, app):
    with app.app_context():
        url = URL(
            short_code='DEVICE2',
            long_url='http://example.com',
            android_target_url='http://android.example.com'
        )
        db.session.add(url)
        db.session.commit()

    headers = {'User-Agent': 'Mozilla/5.0 (Linux; Android 10; SM-G973F) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/80.0.3987.149 Mobile Safari/537.36'}
    response = client.get('/DEVICE2', headers=headers)
    assert b'http://android.example.com' in response.data

def test_redirect_rotation(client, app):
    with app.app_context():
        url = URL(
            short_code='ROTATE',
            long_url='http://example.com',
            rotate_targets=['http://alt1.com', 'http://alt2.com']
        )
        db.session.add(url)
        db.session.commit()

    response = client.get('/ROTATE')
    assert response.status_code == 200
    # It should be one of the three
    assert any(b in response.data for b in [b'http://example.com', b'http://alt1.com', b'http://alt2.com'])

def test_redirect_preview(client, app):
    with app.app_context():
        url = URL(short_code='PREVIEW', long_url='http://example.com', preview_mode=True)
        db.session.add(url)
        db.session.commit()

    response = client.get('/PREVIEW')
    assert response.status_code == 200
    # Check if preview template is used. Usually it has a link to the destination.
    assert b'http://example.com' in response.data
    assert b'Preview' in response.data or b'You are being redirected' in response.data # Assuming typical preview content
