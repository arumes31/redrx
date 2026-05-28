from unittest.mock import MagicMock, patch
from app.utils import get_geo_info
import os

def test_get_geo_info_none(app):
    with app.app_context():
        assert get_geo_info(None) == "Unknown"

def test_get_geo_info_empty(app):
    with app.app_context():
        assert get_geo_info("") == "Unknown"

def test_get_geo_info_invalid(app):
    with app.app_context():
        assert get_geo_info("not-an-ip") == "Unknown"

def test_get_geo_info_local_ipv4(app):
    with app.app_context():
        assert get_geo_info("127.0.0.1") == "Local Network"
        assert get_geo_info("10.0.0.1") == "Local Network"
        assert get_geo_info("192.168.1.1") == "Local Network"
        assert get_geo_info("172.16.0.1") == "Local Network"

def test_get_geo_info_local_ipv6(app):
    with app.app_context():
        assert get_geo_info("::1") == "Local Network"
        assert get_geo_info("fe80::1") == "Local Network"

@patch('app.utils.get_client_country')
def test_get_geo_info_cloudflare(mock_get_country, app):
    mock_get_country.return_value = "US"
    request = MagicMock()
    with app.app_context():
        assert get_geo_info("1.1.1.1", request=request) == "US"

@patch('app.utils._redis_client')
def test_get_geo_info_redis_cache(mock_redis, app):
    mock_redis.get.return_value = "CachedCountry"
    with app.app_context():
        # Redis client is already initialized or mocked here
        assert get_geo_info("8.8.8.8") == "CachedCountry"
        mock_redis.get.assert_called_with("geo:8.8.8.8")

@patch('geoip2.database.Reader')
def test_get_geo_info_db_lookup(mock_reader_cls, app):
    mock_reader = MagicMock()
    mock_reader_cls.return_value.__enter__.return_value = mock_reader

    mock_response = MagicMock()
    mock_response.country.name = "United Kingdom"
    mock_reader.country.return_value = mock_response

    # Ensure DB path exists for the test
    db_path = app.config.get('GEOIP_DB_PATH')
    os.makedirs(os.path.dirname(db_path), exist_ok=True)
    with open(db_path, 'w') as f:
        f.write('dummy content')

    try:
        with app.app_context():
            # Clear redis mock to ensure it doesn't return cached value
            with patch('app.utils._redis_client', None):
                assert get_geo_info("2.2.2.2") == "United Kingdom"
    finally:
        if os.path.exists(db_path):
            os.remove(db_path)

def test_get_geo_info_db_missing(app):
    with app.app_context():
        app.config['GEOIP_DB_PATH'] = '/non/existent/path'
        # Patch redis to None to force DB lookup
        with patch('app.utils._redis_client', None):
            assert get_geo_info("1.1.1.1") == "Unknown (DB Missing)"

@patch('app.utils._get_redis_client')
def test_get_geo_info_redis_failure(mock_get_redis, app):
    mock_redis = MagicMock()
    mock_redis.get.side_effect = Exception("Redis Down")
    mock_get_redis.return_value = mock_redis

    with app.app_context():
        # Should fallback and not crash
        # Since DB is missing in test env by default, it should return Unknown (DB Missing)
        app.config['GEOIP_DB_PATH'] = '/non/existent/path'
        assert get_geo_info("8.8.8.8") == "Unknown (DB Missing)"
