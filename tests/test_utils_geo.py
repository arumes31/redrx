import pytest
from unittest.mock import MagicMock, patch
from app.utils import get_geo_info
import geoip2.errors

def test_get_geo_info_local_network(app):
    with app.app_context():
        # IPv4
        assert get_geo_info('127.0.0.1') == "Local Network"
        assert get_geo_info('192.168.1.1') == "Local Network"
        assert get_geo_info('10.0.0.1') == "Local Network"
        assert get_geo_info('172.16.0.1') == "Local Network"
        # IPv6
        assert get_geo_info('::1') == "Local Network"
        assert get_geo_info('fe80::1') == "Local Network"

def test_get_geo_info_db_missing(app):
    with app.app_context():
        with patch('os.path.exists', return_value=False):
             app.config['GEOIP_DB_PATH'] = '/non/existent/path'
             with patch('app.utils._redis_client', None):
                 assert get_geo_info('8.8.8.8') == "Unknown (DB Missing)"

@patch('app.utils._redis_client')
def test_get_geo_info_redis_cache(mock_redis, app):
    with app.app_context():
        mock_redis.get.return_value = "CachedCountry"
        assert get_geo_info('8.8.8.8') == "CachedCountry"
        mock_redis.get.assert_called_with("geo:8.8.8.8")

@patch('app.utils.get_client_country')
@patch('app.utils._redis_client')
def test_get_geo_info_cloudflare(mock_redis, mock_cf, app):
    with app.app_context():
        mock_redis.get.return_value = None
        mock_cf.return_value = "US"
        request = MagicMock()

        assert get_geo_info('8.8.8.8', request=request) == "US"
        mock_cf.assert_called_with(request)
        # Should also cache it
        mock_redis.setex.assert_called()

def test_get_geo_info_db_lookup(app):
    with app.app_context():
        with patch('geoip2.database.Reader') as mock_reader:
            mock_instance = mock_reader.return_value.__enter__.return_value
            mock_instance.country.return_value.country.name = "TestCountry"

            with patch('os.path.exists', return_value=True):
                with patch('app.utils._redis_client', None):
                    app.config['GEOIP_DB_PATH'] = '/fake/path'
                    assert get_geo_info('8.8.8.8') == "TestCountry"

def test_get_geo_info_db_lookup_not_found(app):
    with app.app_context():
        with patch('geoip2.database.Reader') as mock_reader:
            mock_instance = mock_reader.return_value.__enter__.return_value
            mock_instance.country.side_effect = geoip2.errors.AddressNotFoundError("Not found")

            with patch('os.path.exists', return_value=True):
                with patch('app.utils._redis_client', None):
                    app.config['GEOIP_DB_PATH'] = '/fake/path'
                    assert get_geo_info('8.8.8.8') == "Unknown"

def test_get_geo_info_none_ip(app):
    with app.app_context():
        assert get_geo_info(None) == "Unknown"

def test_get_geo_info_empty_ip(app):
    with app.app_context():
        assert get_geo_info("") == "Unknown"

def test_get_geo_info_invalid_ip(app):
    with app.app_context():
        with patch('app.utils._redis_client', None):
            with patch('os.path.exists', return_value=False):
                assert get_geo_info('not-an-ip') == "Unknown (DB Missing)"
