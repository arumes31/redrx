import pytest
import datetime
from app.models import db, URL

def test_api_shorten_valid_start_at(client, test_user):
    # ISO 8601 format
    start_date = "2025-01-01T12:00:00"
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={
                               'long_url': 'https://google.com',
                               'start_at': start_date
                           })
    assert response.status_code == 201
    data = response.get_json()

    # Check DB
    url = db.session.get(URL, URL.query.filter_by(short_code=data['short_code']).first().id)
    assert url.start_at is not None
    assert url.start_at == datetime.datetime.fromisoformat(start_date)

def test_api_shorten_valid_start_at_z_suffix(client, test_user):
    # ISO 8601 format with Z suffix
    start_date_str = "2025-01-01T12:00:00Z"
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={
                               'long_url': 'https://google.com',
                               'start_at': start_date_str
                           })
    assert response.status_code == 201
    data = response.get_json()

    # Check DB
    url = db.session.get(URL, URL.query.filter_by(short_code=data['short_code']).first().id)
    assert url.start_at is not None
    # fromisoformat with +00:00 should match what we expect
    expected = datetime.datetime.fromisoformat(start_date_str.replace('Z', '+00:00'))
    # Note: Depending on how SQLAlchemy/SQLite handles timezones, we might need to be careful with naive vs aware datetimes.
    # The code uses fromisoformat which might return aware if +00:00 is present.
    assert url.start_at.replace(tzinfo=None) == expected.replace(tzinfo=None)

def test_api_shorten_valid_end_at(client, test_user):
    # ISO 8601 format
    end_date = "2025-12-31T23:59:59"
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={
                               'long_url': 'https://google.com',
                               'end_at': end_date
                           })
    assert response.status_code == 201
    data = response.get_json()

    # Check DB
    url = db.session.get(URL, URL.query.filter_by(short_code=data['short_code']).first().id)
    assert url.end_at is not None
    assert url.end_at == datetime.datetime.fromisoformat(end_date)

def test_api_shorten_invalid_start_at(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={
                               'long_url': 'https://google.com',
                               'start_at': 'invalid-date'
                           })
    assert response.status_code == 400
    assert response.get_json()['error'] == 'Invalid start_at format. Use ISO 8601'

def test_api_shorten_invalid_end_at(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={
                               'long_url': 'https://google.com',
                               'end_at': 'not-a-date'
                           })
    assert response.status_code == 400
    assert response.get_json()['error'] == 'Invalid end_at format. Use ISO 8601'
