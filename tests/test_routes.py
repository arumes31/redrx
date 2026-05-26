import pytest
from app.models import URL, db

def test_index_get(client):
    response = client.get('/')
    assert response.status_code == 200
    assert b'Shorten' in response.data

def test_index_post_success(client, app):
    response = client.post('/', data={
        'long_url': 'https://example.com',
        'custom_code': 'MYCODE'
    }, follow_redirects=True)
    assert response.status_code == 200
    assert b'URL Shortened Successfully!' in response.data

    with app.app_context():
        url = URL.query.filter_by(short_code='MYCODE').first()
        assert url is not None
        assert url.long_url == 'https://example.com'

def test_index_post_invalid_url(client):
    response = client.post('/', data={
        'long_url': 'not-a-url'
    }, follow_redirects=True)
    # WTForms might return 200 with error message
    assert response.status_code == 200
    assert b'Invalid URL' in response.data

def test_index_post_custom_code_taken(client, app):
    with app.app_context():
        url = URL(short_code='TAKEN', long_url='https://google.com')
        db.session.add(url)
        db.session.commit()

    response = client.post('/', data={
        'long_url': 'https://example.com',
        'custom_code': 'TAKEN'
    }, follow_redirects=True)
    # The message is flashed as danger, and should be in the response data
    assert b'TAKEN' in response.data
    assert b'already taken' in response.data

def test_index_post_with_expiry(client, app):
    response = client.post('/', data={
        'long_url': 'https://example.com',
        'expiry_hours': 24
    }, follow_redirects=True)
    assert b'URL Shortened Successfully!' in response.data

    with app.app_context():
        # Find the most recently created URL for this long_url
        url = URL.query.filter_by(long_url='https://example.com').order_by(URL.created_at.desc()).first()
        assert url.expires_at is not None
