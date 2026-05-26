import pytest
from app.models import db, URL
import datetime

def test_api_shorten_start_at_valid(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={
                               'long_url': 'https://example.com',
                               'start_at': '2023-12-01T12:00:00'
                           })
    assert response.status_code == 201
    data = response.get_json()
    assert data['start_at'] == '2023-12-01T12:00:00'

def test_api_shorten_start_at_z_suffix(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={
                               'long_url': 'https://example.com',
                               'start_at': '2023-12-01T12:00:00Z'
                           })
    assert response.status_code == 201
    data = response.get_json()
    assert data['start_at'] == '2023-12-01T12:00:00+00:00'

def test_api_shorten_start_at_invalid(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={
                               'long_url': 'https://example.com',
                               'start_at': 'invalid-date'
                           })
    assert response.status_code == 400
    assert response.get_json()['error'] == 'Invalid start_at format. Use ISO 8601'

def test_api_shorten_end_at_valid(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={
                               'long_url': 'https://example.com',
                               'end_at': '2023-12-31T23:59:59'
                           })
    assert response.status_code == 201
    data = response.get_json()
    assert data['end_at'] == '2023-12-31T23:59:59'

def test_api_shorten_end_at_z_suffix(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={
                               'long_url': 'https://example.com',
                               'end_at': '2023-12-31T23:59:59Z'
                           })
    assert response.status_code == 201
    data = response.get_json()
    assert data['end_at'] == '2023-12-31T23:59:59+00:00'

def test_api_shorten_end_at_invalid(client, test_user):
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={
                               'long_url': 'https://example.com',
                               'end_at': 'not-a-date'
                           })
    assert response.status_code == 400
    assert response.get_json()['error'] == 'Invalid end_at format. Use ISO 8601'

def test_api_shorten_dates_saved(app, client, test_user):
    start_str = '2024-01-01T00:00:00Z'
    end_str = '2024-12-31T23:59:59Z'
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={
                               'long_url': 'https://example.com',
                               'start_at': start_str,
                               'end_at': end_str
                           })
    assert response.status_code == 201
    data = response.get_json()
    short_code = data['short_code']

    with app.app_context():
        url = URL.query.filter_by(short_code=short_code).first()
        assert url is not None
        assert url.start_at is not None
        assert url.end_at is not None
        assert url.start_at.year == 2024
        assert url.end_at.year == 2024
