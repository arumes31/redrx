from app.models import db, URL
import datetime

def test_index_get(client):
    response = client.get('/')
    assert response.status_code == 200
    assert b'Shorten' in response.data

def test_index_post_basic(client, app):
    response = client.post('/', data={
        'long_url': 'https://example.com'
    }, follow_redirects=True)
    assert response.status_code == 200
    assert b'URL Shortened Successfully!' in response.data
    with app.app_context():
        url = URL.query.filter_by(long_url='https://example.com').first()
        assert url is not None

def test_index_post_custom_code(client, app):
    response = client.post('/', data={
        'long_url': 'https://example.com',
        'custom_code': 'MYCODE'
    }, follow_redirects=True)
    assert response.status_code == 200
    assert b'URL Shortened Successfully!' in response.data
    with app.app_context():
        url = URL.query.filter_by(short_code='MYCODE').first()
        assert url is not None

def test_index_post_taken_custom_code(client, app):
    with app.app_context():
        u = URL(short_code='TAKEN', long_url='https://taken.com')
        db.session.add(u)
        db.session.commit()

    response = client.post('/', data={
        'long_url': 'https://example.com',
        'custom_code': 'TAKEN'
    }, follow_redirects=True)
    assert response.status_code == 200
    assert b"already taken" in response.data

def test_index_post_invalid_url(client):
    response = client.post('/', data={
        'long_url': 'not-a-url'
    }, follow_redirects=True)
    assert response.status_code == 200
    assert b"Invalid URL" in response.data

def test_index_post_advanced_options(client, app):
    response = client.post('/', data={
        'long_url': 'https://example.com',
        'rotate_targets': 'https://alt1.com, https://alt2.com',
        'ios_target_url': 'https://ios.com',
        'android_target_url': 'https://android.com',
        'expiry_hours': 10
    }, follow_redirects=True)
    assert response.status_code == 200
    assert b'URL Shortened Successfully!' in response.data
    with app.app_context():
        url = URL.query.filter_by(long_url='https://example.com').first()
        assert url is not None
        assert url.rotate_targets == ['https://alt1.com', 'https://alt2.com']
        assert url.ios_target_url == 'https://ios.com'
        assert url.android_target_url == 'https://android.com'
        assert url.expires_at is not None

def test_index_post_too_many_rotate_targets(client):
    targets = ','.join(['https://ex.com'] * 51)
    response = client.post('/', data={
        'long_url': 'https://example.com',
        'rotate_targets': targets
    }, follow_redirects=True)
    assert response.status_code == 200
    assert b"Maximum 50 rotate targets allowed" in response.data

def test_index_anonymous_limit_long_expiry(client, app):
    # For unauthenticated users, expiry > 8760 (1 year) or 0 (permanent) should flash a warning
    response = client.post('/', data={
        'long_url': 'https://example.com',
        'expiry_hours': 0
    }, follow_redirects=True)
    assert b"Please log in to create links longer than 1 year or permanent links" in response.data

    response = client.post('/', data={
        'long_url': 'https://example.com',
        'expiry_hours': 8761
    }, follow_redirects=True)
    assert b"Please log in to create links longer than 1 year or permanent links" in response.data

def test_index_disable_anonymous(client, app):
    app.config['DISABLE_ANONYMOUS_CREATE'] = True
    response = client.post('/', data={
        'long_url': 'https://example.com'
    }, follow_redirects=True)
    assert b"Please log in to shorten URLs" in response.data
    app.config['DISABLE_ANONYMOUS_CREATE'] = False

def test_index_post_timestamps(client, app):
    # Test scheduling
    today = datetime.date.today()
    start_date = (today + datetime.timedelta(days=1)).isoformat()
    end_date = (today + datetime.timedelta(days=2)).isoformat()
    response = client.post('/', data={
        'long_url': 'https://example.com',
        'start_date': start_date,
        'start_time': '12:00',
        'end_date': end_date,
        'end_time': '12:00'
    }, follow_redirects=True)
    assert b'URL Shortened Successfully!' in response.data
    with app.app_context():
        url = URL.query.filter_by(long_url='https://example.com').first()
        assert url.start_at is not None
        assert url.end_at is not None

def test_index_post_invalid_timestamps(client):
    today = datetime.date.today()
    start_date = (today + datetime.timedelta(days=2)).isoformat()
    end_date = (today + datetime.timedelta(days=1)).isoformat()
    response = client.post('/', data={
        'long_url': 'https://example.com',
        'start_date': start_date,
        'start_time': '12:00',
        'end_date': end_date,
        'end_time': '12:00'
    }, follow_redirects=True)
    assert b"End time must be after start time" in response.data
