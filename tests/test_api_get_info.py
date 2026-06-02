from app.models import db, URL, User
import datetime

def test_api_get_info_success(app, client, test_user):
    # Create a URL first
    with app.app_context():
        url = URL(short_code='TESTCODE', long_url='https://google.com', user_id=test_user.id)
        db.session.add(url)
        db.session.commit()

    response = client.get('/api/v1/TESTCODE',
                          headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 200
    data = response.get_json()
    assert data['short_code'] == 'TESTCODE'
    assert data['long_url'] == 'https://google.com'
    assert data['clicks_count'] == 0
    assert 'created_at' in data
    assert data['active'] is True

def test_api_get_info_all_fields(app, client, test_user):
    expires = datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(days=1)
    with app.app_context():
        url = URL(
            short_code='FULLINFO',
            long_url='https://example.com',
            user_id=test_user.id,
            expires_at=expires
        )
        db.session.add(url)
        db.session.commit()

    response = client.get('/api/v1/FULLINFO',
                          headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 200
    data = response.get_json()
    assert data['short_code'] == 'FULLINFO'
    assert data['long_url'] == 'https://example.com'
    assert data['clicks_count'] == 0
    assert 'created_at' in data
    assert data['expires_at'] is not None
    assert data['active'] is True

def test_api_get_info_unauthorized_owner(app, client, test_user):
    # Create another user
    with app.app_context():
        other_user = User(username='other', email='other@example.com', password_hash='hash', api_key='other-key')
        db.session.add(other_user)
        db.session.commit()

        url = URL(short_code='OTHERURL', long_url='https://google.com', user_id=other_user.id)
        db.session.add(url)
        db.session.commit()

    # Try to access other user's URL with test_user's key
    response = client.get('/api/v1/OTHERURL',
                          headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 403
    assert response.get_json()['error'] == 'Access denied. You do not own this URL.'

def test_api_get_info_case_insensitive(app, client, test_user):
    # Create a URL first
    with app.app_context():
        url = URL(short_code='TESTCODE', long_url='https://google.com', user_id=test_user.id)
        db.session.add(url)
        db.session.commit()

    # Query with lowercase short code
    response = client.get('/api/v1/testcode',
                          headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 200
    data = response.get_json()
    assert data['short_code'] == 'TESTCODE'

def test_api_get_info_not_found(client, test_user):
    response = client.get('/api/v1/NONEXISTENT',
                          headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 404
    assert response.get_json()['error'] == 'URL not found'

def test_api_get_info_unauthorized_missing_key(client):
    response = client.get('/api/v1/TESTCODE')
    assert response.status_code == 401
    assert response.get_json()['error'] == 'Valid API Key required. Access denied.'

def test_api_get_info_unauthorized_invalid_key(client):
    response = client.get('/api/v1/TESTCODE',
                          headers={'X-API-KEY': 'invalid-key'})
    assert response.status_code == 401
    assert response.get_json()['error'] == 'Valid API Key required. Access denied.'

def test_api_get_info_inactive_url(app, client, test_user):
    # Create an inactive URL (expired)
    with app.app_context():
        url = URL(
            short_code='EXPIRED',
            long_url='https://google.com',
            user_id=test_user.id,
            expires_at=datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(hours=1)
        )
        db.session.add(url)
        db.session.commit()

    response = client.get('/api/v1/EXPIRED',
                          headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 200
    data = response.get_json()
    assert data['short_code'] == 'EXPIRED'
    assert data['active'] is False
