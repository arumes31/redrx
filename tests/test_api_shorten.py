import datetime

def test_api_shorten_valid_dates(client, test_user):
    start_at = (datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=1)).isoformat()
    end_at = (datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=2)).isoformat()

    # Test with Z and +00:00
    start_at_z = start_at.replace('+00:00', 'Z')

    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={
                               'long_url': 'https://example.com',
                               'start_at': start_at_z,
                               'end_at': end_at
                           })

    assert response.status_code == 201
    data = response.get_json()
    assert data['start_at'] is not None
    assert data['end_at'] is not None
    # fromisoformat handles both Z (if replaced by +00:00 in code) and +00:00
    assert data['start_at'].startswith(start_at_z[:-1]) or data['start_at'] == start_at
    assert data['end_at'] == end_at

def test_api_shorten_invalid_start_at(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={
                               'long_url': 'https://example.com',
                               'start_at': 'invalid-date'
                           })

    assert response.status_code == 400
    assert response.get_json()['error'] == 'Invalid start_at format. Use ISO 8601'

def test_api_shorten_invalid_end_at(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={
                               'long_url': 'https://example.com',
                               'end_at': 'not-a-date'
                           })

    assert response.status_code == 400
    assert response.get_json()['error'] == 'Invalid end_at format. Use ISO 8601'

def test_api_shorten_no_dates(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={
                               'long_url': 'https://example.com'
                           })

    assert response.status_code == 201
    data = response.get_json()
    assert data['start_at'] is None
    assert data['end_at'] is None
