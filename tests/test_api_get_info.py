from app.models import db, URL

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
    import datetime
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

def test_api_get_info_with_clicks(app, client, test_user):
    with app.app_context():
        url = URL(short_code='CLICKS', long_url='https://google.com', user_id=test_user.id)
        url.clicks_count = 15
        db.session.add(url)
        db.session.commit()

    response = client.get('/api/v1/CLICKS', headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 200
    assert response.get_json()['clicks_count'] == 15

def test_api_get_info_disabled(app, client, test_user):
    with app.app_context():
        url = URL(short_code='DISABLED', long_url='https://google.com', user_id=test_user.id, is_enabled=False)
        db.session.add(url)
        db.session.commit()

    response = client.get('/api/v1/DISABLED', headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 200
    assert response.get_json()['active'] is False

def test_api_get_info_scheduled_future(app, client, test_user):
    import datetime
    future = datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(days=1)
    future = future.replace(tzinfo=None)
    with app.app_context():
        url = URL(short_code='FUTURE', long_url='https://google.com', user_id=test_user.id, start_at=future)
        db.session.add(url)
        db.session.commit()

    response = client.get('/api/v1/FUTURE', headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 200
    assert response.get_json()['active'] is False

def test_api_get_info_scheduled_past(app, client, test_user):
    import datetime
    past = datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(days=1)
    past = past.replace(tzinfo=None)
    with app.app_context():
        url = URL(short_code='PAST', long_url='https://google.com', user_id=test_user.id, end_at=past)
        db.session.add(url)
        db.session.commit()

    response = client.get('/api/v1/PAST', headers={'X-API-KEY': 'test-api-key'})
    assert response.status_code == 200
    assert response.get_json()['active'] is False
