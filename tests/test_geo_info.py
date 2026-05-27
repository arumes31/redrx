import pytest
from unittest.mock import MagicMock, patch
from flask import Flask
from app.utils import get_geo_info

@pytest.fixture
def app():
    app = Flask(__name__)
    app.config['GEOIP_DB_PATH'] = '/path/to/db'
    return app

@pytest.fixture
def mock_redis():
    with patch('app.utils._redis_client') as mock_r:
        yield mock_r

def test_get_geo_info_cached(app, mock_redis):
    with app.app_context():
        mock_redis.get.return_value = "CachedCountry"
        result = get_geo_info("1.2.3.4")
        assert result == "CachedCountry"
        mock_redis.get.assert_called_with("geo:1.2.3.4")

def test_get_geo_info_cloudflare(app, mock_redis):
    with app.app_context():
        mock_redis.get.return_value = None
        mock_request = MagicMock()

        with patch('app.utils.get_client_country') as mock_get_cf:
            mock_get_cf.return_value = "CFCountry"
            result = get_geo_info("1.2.3.4", request=mock_request)

        assert result == "CFCountry"
        mock_redis.setex.assert_called_with("geo:1.2.3.4", 300, "CFCountry")

def test_get_geo_info_local_network(app, mock_redis):
    with app.app_context():
        mock_redis.get.return_value = None
        assert get_geo_info("127.0.0.1") == "Local Network"
        assert get_geo_info("192.168.1.1") == "Local Network"
        assert get_geo_info("10.0.0.1") == "Local Network"
        assert get_geo_info("172.16.0.1") == "Local Network"

def test_get_geo_info_local_db(app, mock_redis):
    with app.app_context():
        mock_redis.get.return_value = None

        with patch('os.path.exists') as mock_exists,              patch('geoip2.database.Reader') as mock_reader:
            mock_exists.return_value = True
            mock_response = MagicMock()
            mock_response.country.name = "LocalDBCountry"
            mock_reader.return_value.__enter__.return_value.country.return_value = mock_response

            result = get_geo_info("8.8.8.8")

        assert result == "LocalDBCountry"
        mock_redis.setex.assert_called_with("geo:8.8.8.8", 300, "LocalDBCountry")

def test_get_geo_info_db_missing(app, mock_redis):
    with app.app_context():
        mock_redis.get.return_value = None
        with patch('os.path.exists') as mock_exists:
            mock_exists.return_value = False
            result = get_geo_info("8.8.8.8")
        assert result == "Unknown (DB Missing)"
