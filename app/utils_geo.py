import os
import geoip2.database
from flask import current_app

_GEO_PREFIX = "geo:"
_GEO_CACHE_TTL = 300  # 5 minutes

def _get_cached_geo(ip, redis_client):
    """Retrieves geo info from Redis cache."""
    if redis_client:
        try:
            return redis_client.get(f"{_GEO_PREFIX}{ip}")
        except Exception:
            pass
    return None

def _set_cached_geo(ip, country, redis_client):
    """Sets geo info in Redis cache."""
    if redis_client:
        try:
            redis_client.setex(f"{_GEO_PREFIX}{ip}", _GEO_CACHE_TTL, country)
        except Exception:
            pass

def _is_local_ip(ip):
    """Checks if the IP is a local network address."""
    return ip == "127.0.0.1" or ip.startswith("192.168.") or ip.startswith("10.") or ip.startswith("172.")

def _get_local_db_geo(ip):
    """Fetches country from local MaxMind database."""
    db_path = current_app.config.get("GEOIP_DB_PATH")
    if not db_path or not os.path.exists(db_path):
        return "Unknown (DB Missing)"

    try:
        with geoip2.database.Reader(db_path) as reader:
            response = reader.country(ip)
            return response.country.name or "Unknown"
    except Exception:
        return "Unknown"
