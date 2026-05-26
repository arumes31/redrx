import pytest
from unittest.mock import MagicMock, patch
from app.utils import get_geo_info

def test_get_geo_info_cache_hit(app):
    with patch('app.utils._redis_client') as mock_redis:
        mock_redis.get.return_value = 'CachedCountry'
        result = get_geo_info('1.2.3.4')
        assert result == 'CachedCountry'
        mock_redis.get.assert_called_with('geo:1.2.3.4')

def test_get_geo_info_cloudflare_hit(app):
    with patch('app.utils._redis_client') as mock_redis,          patch('app.utils.get_client_country') as mock_cf:
        mock_redis.get.return_value = None
        mock_cf.return_value = 'CFCountry'

        request = MagicMock()
        result = get_geo_info('1.2.3.4', request=request)

        assert result == 'CFCountry'
        mock_cf.assert_called_with(request)
        mock_redis.setex.assert_called()

def test_get_geo_info_local_network(app):
    with patch('app.utils._redis_client') as mock_redis:
        mock_redis.get.return_value = None

        assert get_geo_info('127.0.0.1') == 'Local Network'
        assert get_geo_info('192.168.1.1') == 'Local Network'
        assert get_geo_info('10.0.0.1') == 'Local Network'
        assert get_geo_info('172.16.0.1') == 'Local Network'

def test_get_geo_info_db_lookup(app):
    with patch('app.utils._redis_client') as mock_redis,          patch('geoip2.database.Reader') as mock_reader,          patch('os.path.exists') as mock_exists:

        mock_redis.get.return_value = None
        mock_exists.return_value = True

        mock_instance = mock_reader.return_value.__enter__.return_value
        mock_instance.country.return_value.country.name = 'DBCountry'

        with app.app_context():
            app.config['GEOIP_DB_PATH'] = '/fake/path.mmdb'
            result = get_geo_info('8.8.8.8')

        assert result == 'DBCountry'
        mock_redis.setex.assert_called_with('geo:8.8.8.8', 300, 'DBCountry')

def test_get_geo_info_db_missing(app):
    with patch('app.utils._redis_client') as mock_redis,          patch('os.path.exists') as mock_exists:

        mock_redis.get.return_value = None
        mock_exists.return_value = False

        with app.app_context():
            app.config['GEOIP_DB_PATH'] = '/fake/path.mmdb'
            result = get_geo_info('8.8.8.8')

        assert result == 'Unknown (DB Missing)'
