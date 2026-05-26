import pytest
from unittest.mock import MagicMock, patch
from app.utils import get_geo_info


def test_get_geo_info_local_ips(app):
    with app.app_context():
        # IPv4 Loopback
        assert get_geo_info("127.0.0.1") == "Local Network"
        # IPv4 Private
        assert get_geo_info("10.0.0.1") == "Local Network"
        assert get_geo_info("192.168.1.1") == "Local Network"
        assert get_geo_info("172.16.0.1") == "Local Network"

        # IPv6 Loopback
        assert get_geo_info("::1") == "Local Network"
        # IPv6 Private
        assert get_geo_info("fd00::1") == "Local Network"


def test_get_geo_info_db_missing(app):
    with app.app_context():
        with patch("os.path.exists", return_value=False):
            # This should bypass Redis and Cloudflare and reach the DB check
            assert get_geo_info("8.8.8.8") == "Unknown (DB Missing)"


def test_get_geo_info_cloudflare_header(app):
    with app.app_context():
        app.config["USE_CLOUDFLARE"] = True
        request = MagicMock()
        request.headers = {"CF-IPCountry": "US"}

        # Mock redis to avoid actual connection
        with patch("app.utils._redis_client", None):
            assert get_geo_info("8.8.8.8", request=request) == "US"


def test_get_geo_info_invalid_ip(app):
    with app.app_context():
        assert get_geo_info("not-an-ip") == "Invalid IP"
        assert get_geo_info("999.999.999.999") == "Invalid IP"


def test_get_geo_info_redis_cache(app):
    with app.app_context():
        mock_redis = MagicMock()
        mock_redis.get.return_value = "CachedCountry"

        with patch("app.utils._redis_client", mock_redis):
            assert get_geo_info("8.8.8.8") == "CachedCountry"
            mock_redis.get.assert_called_with("geo:8.8.8.8")


def test_get_geo_info_redis_fallback(app):
    with app.app_context():
        mock_redis = MagicMock()
        mock_redis.get.side_effect = Exception("Redis Down")

        with patch("app.utils._redis_client", mock_redis):
            with patch("os.path.exists", return_value=False):
                # Should fallback to DB Missing because Redis failed
                assert get_geo_info("8.8.8.8") == "Unknown (DB Missing)"
