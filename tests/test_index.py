from app.models import URL, db

def test_index_get(client):
    response = client.get('/')
    assert response.status_code == 200
    assert b'Shorten' in response.data

def test_index_post_success(client, app):
    with app.app_context():
        response = client.post('/', data={
            'long_url': 'https://google.com',
            'code_length': 6
        }, follow_redirects=True)
        assert response.status_code == 200
        assert b'URL Shortened Successfully!' in response.data

        url = URL.query.filter_by(long_url='https://google.com').first()
        assert url is not None
        assert len(url.short_code) == 6

def test_index_post_custom_code(client, app):
    with app.app_context():
        response = client.post('/', data={
            'long_url': 'https://google.com',
            'custom_code': 'GOOGLE'
        }, follow_redirects=True)
        assert response.status_code == 200
        assert b'URL Shortened Successfully!' in response.data

        url = URL.query.filter_by(short_code='GOOGLE').first()
        assert url is not None

def test_index_post_custom_code_taken(client, app):
    with app.app_context():
        url = URL(short_code='TAKEN', long_url='https://example.com')
        db.session.add(url)
        db.session.commit()

        response = client.post('/', data={
            'long_url': 'https://google.com',
            'custom_code': 'TAKEN'
        }, follow_redirects=True)
        assert response.status_code == 200
        assert b"is already taken" in response.data

def test_index_post_unsafe_url(client, app):
    import os
    os.environ['BLOCKED_DOMAINS'] = 'evil.com'

    response = client.post('/', data={
        'long_url': 'https://evil.com'
    }, follow_redirects=True)
    assert response.status_code == 200
    assert b"is blocked for safety reasons" in response.data

def test_index_post_expiry(client, app):
    with app.app_context():
        response = client.post('/', data={
            'long_url': 'https://google.com',
            'expiry_hours': 24
        }, follow_redirects=True)
        assert response.status_code == 200
        url = URL.query.filter_by(long_url='https://google.com').first()
        assert url.expires_at is not None
