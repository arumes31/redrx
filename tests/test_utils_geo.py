import pytest
from unittest.mock import MagicMock, patch
from app.utils import get_geo_info

@pytest.fixture
def mock_redis():
    with patch('app.utils._redis_client') as mock:
        yield mock

@pytest.fixture
def mock_geoip():
    with patch('geoip2.database.Reader') as mock:
        yield mock

def test_get_geo_info_redis_hit(app, mock_redis):
    mock_redis.get.return_value = "CachedCountry"

    result = get_geo_info("1.2.3.4")

    assert result == "CachedCountry"
    mock_redis.get.assert_called_with("geo:1.2.3.4")

def test_get_geo_info_cloudflare_hit(app, mock_redis):
    mock_redis.get.return_value = None
    mock_request = MagicMock()
    mock_request.headers = {'CF-IPCountry': 'US'}

    with patch('app.utils.current_app') as mock_app:
        mock_app.config = {'USE_CLOUDFLARE': True}
        result = get_geo_info("1.2.3.4", request=mock_request)

    assert result == "US"
    mock_redis.setex.assert_called_with("geo:1.2.3.4", 300, "US")

def test_get_geo_info_local_network(app, mock_redis):
    mock_redis.get.return_value = None

    assert get_geo_info("127.0.0.1") == "Local Network"
    assert get_geo_info("192.168.1.1") == "Local Network"
    assert get_geo_info("10.0.0.1") == "Local Network"
    assert get_geo_info("172.16.0.1") == "Local Network"

def test_get_geo_info_db_lookup(app, mock_redis, mock_geoip):
    mock_redis.get.return_value = None
    mock_reader = mock_geoip.return_value.__enter__.return_value
    mock_response = MagicMock()
    mock_response.country.name = "United States"
    mock_reader.country.return_value = mock_response

    with patch('app.utils.current_app') as mock_app:
        mock_app.config = {'GEOIP_DB_PATH': '/tmp/test.mmdb'}
        with patch('os.path.exists', return_value=True):
            result = get_geo_info("8.8.8.8")

    assert result == "United States"
    mock_redis.setex.assert_called_with("geo:8.8.8.8", 300, "United States")

def test_get_geo_info_db_missing(app, mock_redis):
    mock_redis.get.return_value = None

    with patch('app.utils.current_app') as mock_app:
        mock_app.config = {'GEOIP_DB_PATH': '/nonexistent.mmdb'}
        with patch('os.path.exists', return_value=False):
            result = get_geo_info("8.8.8.8")

    assert result == "Unknown (DB Missing)"

def test_get_geo_info_redis_error(app, mock_redis, mock_geoip):
    mock_redis.get.side_effect = Exception("Redis Down")
    mock_reader = mock_geoip.return_value.__enter__.return_value
    mock_response = MagicMock()
    mock_response.country.name = "United Kingdom"
    mock_reader.country.return_value = mock_response

    with patch('app.utils.current_app') as mock_app:
        mock_app.config = {'GEOIP_DB_PATH': '/tmp/test.mmdb'}
        with patch('os.path.exists', return_value=True):
            result = get_geo_info("8.8.8.8")

    assert result == "United Kingdom"
