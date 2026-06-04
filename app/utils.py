import uuid
import ipaddress
import qrcode
import io
import datetime
import base64
from PIL import Image

import requests
import geoip2.database
from flask import current_app
from concurrent.futures import ThreadPoolExecutor
import time

import os
from urllib.parse import urlparse
from flask import request
import redis

# Redis cache configuration for GeoIP lookups
_REDIS_URL = os.environ.get('RATELIMIT_STORAGE_URL', 'redis://localhost:6379')
_GEO_CACHE_TTL = 300  # 5 minutes
_GEO_PREFIX = "geo:"

def _get_redis_client():
    """Lazily initialize Redis client."""
    if _REDIS_URL.startswith('redis://'):
        try:
            return redis.from_url(_REDIS_URL, decode_responses=True)
        except Exception:
            return None
    return None

_redis_client = None

_blocked_domains_cache = None
_blocked_domains_mtime = 0
_blocked_domains_path = None
_blocked_domains_ino = 0


_blocked_env_cache = set()
_blocked_env_raw = None

def _get_blocked_env_set():
    """Returns a set of blocked domains from the environment config, cached for performance."""
    global _blocked_env_cache, _blocked_env_raw
    blocked_env = current_app.config.get("BLOCKED_DOMAINS", [])

    if blocked_env != _blocked_env_raw:
        _blocked_env_cache = {str(b).lower() for b in blocked_env if b}
        _blocked_env_raw = list(blocked_env)

    return _blocked_env_cache

def update_phishing_list():
    """Downloads the latest phishing domain lists."""
    if not current_app.config.get('ENABLE_PHISHING_CHECK'):
        return

    urls = current_app.config.get('PHISHING_LIST_URLS')
    path = current_app.config.get('BLOCKED_DOMAINS_PATH')
    interval = current_app.config.get('PHISHING_CHECK_INTERVAL', 24)
    
    if not urls or not path:
        return
    
    try:
        # Check if file is old (e.g. older than interval hours)
        if os.path.exists(path):
            file_age = time.time() - os.path.getmtime(path)
            if file_age < (interval * 3600):
                return

        def fetch_url(url):
            url = url.strip()
            if not url:
                return None
            try:
                response = requests.get(url, timeout=10)
                if response.status_code == 200:
                    return response.text
            except Exception: # nosec B112
                pass
            return None

        with ThreadPoolExecutor(max_workers=min(len(urls), 10)) as executor:
            results = list(executor.map(fetch_url, urls))

        with open(path, 'w', encoding='utf-8') as f:
            for content in results:
                if content:
                    f.write(content)
                    f.write('\n') # Ensure newline between lists
    except Exception: # nosec B110
        pass

def _check_domain_phishing(domain, blocked_domains):
    """Checks if a domain or any of its suffixes are in the blocked domains set."""
    if not domain or not blocked_domains:
        return False

    if ':' in domain:
        domain = domain.split(':', 1)[0]

    check_domain = domain
    while True:
        if check_domain in blocked_domains:
            return True

        dot_idx = check_domain.find('.')
        if dot_idx == -1:
            break
        check_domain = check_domain[dot_idx + 1:]

    return False

def _is_url_entry_phishing(url_entry, blocked_domains):
    """Checks if the main url or any of its rotate targets are phishing."""
    domain = urlparse(url_entry.long_url).netloc.lower()
    if domain and _check_domain_phishing(domain, blocked_domains):
        return True

    if url_entry.rotate_targets:
        for target in url_entry.rotate_targets:
            target_domain = urlparse(target).netloc.lower()
            if target_domain and _check_domain_phishing(target_domain, blocked_domains):
                return True
    return False

def cleanup_phishing_urls():
    """Removes URLs from database that are found in the phishing lists."""
    if not current_app.config.get('ENABLE_AUTO_REMOVE_PHISHING'):
        return

    path = current_app.config.get('BLOCKED_DOMAINS_PATH')
    if not path or not os.path.exists(path):
        return

    from app.models import db, URL
    
    try:
        with open(path, 'r', encoding='utf-8') as f:
            blocked_domains = {line.strip().lower() for line in f if line.strip()}

        if not blocked_domains:
            return

        urls = URL.query.yield_per(100)
        removed_count = 0
        
        for url_entry in urls:
            try:
                if _is_url_entry_phishing(url_entry, blocked_domains):
                    db.session.delete(url_entry)
                    removed_count += 1
                else:
                    db.session.expunge(url_entry)
            except Exception: # nosec B112
                continue
        
        if removed_count > 0:
            db.session.commit()
            
    except Exception: # nosec B110
        try:
            db.session.rollback()
        except Exception:
            pass

def get_blocked_domains():
    """Returns the set of blocked domains using the in-process cache."""
    if not current_app.config.get('ENABLE_PHISHING_CHECK'):
        return set()

    path = current_app.config.get('BLOCKED_DOMAINS_PATH')
    global _blocked_domains_cache, _blocked_domains_mtime, _blocked_domains_path, _blocked_domains_ino

    if path and os.path.exists(path):
        try:
            stat_info = os.stat(path)
            mtime = stat_info.st_mtime
            ino = stat_info.st_ino

            if (_blocked_domains_cache is None or
                path != _blocked_domains_path or
                mtime != _blocked_domains_mtime or
                ino != _blocked_domains_ino):
                with open(path, 'r', encoding='utf-8') as f:
                    _blocked_domains_cache = {line.strip().lower() for line in f if line.strip()}
                _blocked_domains_mtime = mtime
                _blocked_domains_path = path
                _blocked_domains_ino = ino
            return _blocked_domains_cache
        except Exception as e:
            current_app.logger.error(f"Failed to load blocked domains list: {e}")
            if _blocked_domains_cache is not None:
                return _blocked_domains_cache
            # Fail closed if we cannot read the list and have no cache
            raise RuntimeError(f"Critical Security Failure: Unable to load blocked domains list: {e}")

    elif path:
        if _blocked_domains_cache is not None:
            return _blocked_domains_cache
        raise RuntimeError(f"Critical Security Failure: Blocked domains list configured at '{path}' but file does not exist.")

    # If path is not set at all
    if _blocked_domains_cache is not None:
        return _blocked_domains_cache
    return set()

def is_safe_url(target_url, blocked_domains_cache=None):
    """Checks if the URL is not in the blocked domains list."""
    if not isinstance(target_url, str):
        return False

    # 0. Check URL scheme
    try:
        parsed = urlparse(target_url)
        if parsed.scheme.lower() not in ['http', 'https']:
            return False
    except (ValueError, TypeError):
        return False

    # 1. Check manual overrides from config
    try:
        domain = urlparse(target_url).netloc.lower()
        if not domain: # For relative or malformed URLs
             return False
             
        blocked_env_set = _get_blocked_env_set()
        if _check_domain_phishing(domain, blocked_env_set):
            return False
    except Exception:
        return False

    # 2. Check downloaded list
    if not current_app.config.get('ENABLE_PHISHING_CHECK'):
        return True

    blocked_domains = blocked_domains_cache if blocked_domains_cache is not None else get_blocked_domains()
    if blocked_domains:
        if _check_domain_phishing(domain, blocked_domains):
            return False
            
    return True

def get_client_ip(request):
    """Returns the client IP, preferring Cloudflare header if enabled."""
    if current_app.config.get('USE_CLOUDFLARE'):
        cf_ip = request.headers.get('CF-Connecting-IP')
        if cf_ip:
            return cf_ip
    return request.remote_addr

def get_client_country(request):
    """Returns the client country code, preferring Cloudflare header if enabled."""
    if current_app.config.get('USE_CLOUDFLARE'):
        cf_country = request.headers.get('CF-IPCountry')
        if cf_country:
            return cf_country
    return None

def _get_cached_geo(ip):
    """Retrieves geo info from Redis cache."""
    global _redis_client
    if _redis_client is None:
        _redis_client = _get_redis_client()

    if _redis_client:
        try:
            return _redis_client.get(f"{_GEO_PREFIX}{ip}")
        except Exception:
            pass
    return None

def _set_cached_geo(ip, country):
    """Updates Redis cache with geo info."""
    global _redis_client
    if _redis_client is None:
        _redis_client = _get_redis_client()

    if _redis_client and country:
        try:
            _redis_client.set(f"{_GEO_PREFIX}{ip}", country, ex=_GEO_CACHE_TTL)
        except Exception:
            pass

def _is_local_ip(ip):
    """Checks if an IP address is part of a local or private network."""
    try:
        ip_obj = ipaddress.ip_address(ip)
        return ip_obj.is_private or ip_obj.is_loopback
    except ValueError:
        return False

def _get_db_geo(ip):
    """Fetches country from local MaxMind database."""
    db_path = current_app.config.get('GEOIP_DB_PATH')
    if not db_path or not os.path.exists(db_path):
        return "Unknown (DB Missing)"

    try:
        with geoip2.database.Reader(db_path) as reader:
            response = reader.country(ip)
            return response.country.name or "Unknown"
    except Exception:
        return "Unknown"

def get_geo_info(ip, request=None):
    """Fetches country from IP using local MaxMind database or Cloudflare header with Redis cache."""
    # 1. Check Redis Cache
    cached_val = _get_cached_geo(ip)
    if cached_val:
        return cached_val

    # 2. Check Cloudflare
    if request:
        cf_country = get_client_country(request)
        if cf_country:
            _set_cached_geo(ip, cf_country)
            return cf_country

    # 3. Check Local Network
    if _is_local_ip(ip):
        return "Local Network"
    
    # 4. Check Local DB
    country = _get_db_geo(ip)

    # 5. Update Redis Cache (if not Unknown/Missing)
    if country and "Unknown" not in country:
        _set_cached_geo(ip, country)

    return country

def generate_short_code(length=6):
    """Generates a random short code."""
    return str(uuid.uuid4())[:length].upper()

def generate_qr(data, color='black', bg='white', logo_img=None):
    """Generates a QR code image as a BytesIO object."""
    qr = qrcode.QRCode(version=1, box_size=10, border=5)
    qr.add_data(data)
    qr.make(fit=True)
    
    # Basic color validation/fallback could go here if needed, 
    # but PIL handles standard color names and hex codes well.
    try:
        img = qr.make_image(fill_color=color, back_color=bg).convert('RGB')
    except ValueError:
        # Fallback to defaults on error
        img = qr.make_image(fill_color='black', back_color='white').convert('RGB')

    if logo_img:
        # Ensure logo is compatible
        logo = logo_img.convert("RGBA")
        
        # Resize logo to max 20% of QR size
        max_size = (img.size[0] // 5, img.size[1] // 5)
        logo.thumbnail(max_size, Image.Resampling.LANCZOS)
        
        # Calculate position (center)
        pos = ((img.size[0] - logo.size[0]) // 2, (img.size[1] - logo.size[1]) // 2)
        
        # Create a mask for transparency if needed, but pasting directly works for RGBA on RGB
        img.paste(logo, pos, mask=logo)

    img_buffer = io.BytesIO()
    img.save(img_buffer, format='PNG')
    img_buffer.seek(0)
    return img_buffer

def select_rotate_target(rotate_targets):
    """Selects an alternate URL based on a simple rotation (hash of timestamp)."""
    if not rotate_targets:
        return None

    if isinstance(rotate_targets, str):
        return rotate_targets

    if not isinstance(rotate_targets, list):
        rotate_targets = list(rotate_targets)

    # Using microsecond for more "random" feel on rapid refreshes
    idx = hash(str(datetime.datetime.now().microsecond)) % len(rotate_targets)
    return rotate_targets[idx]

def get_qr_data_url(data, color='black', bg='white', logo_img=None):
    """Returns a base64 encoded data URL for the QR code."""
    img_buffer = generate_qr(data, color, bg, logo_img)
    return base64.b64encode(img_buffer.read()).decode()

def is_safe_redirect_url(target):
    """Checks if a URL is safe for redirection (i.e., it's a relative URL or matches the base domain)."""
    if not target:
        return False
    ref_url = urlparse(request.host_url)
    test_url = urlparse(target)
    if test_url.scheme or test_url.netloc:
        return test_url.scheme == ref_url.scheme and test_url.netloc == ref_url.netloc
    return True

def sanitize_csv_field(field):
    """Sanitizes a field for CSV export to prevent formula injection."""
    if field is None:
        return ""
    str_field = str(field)
    if str_field and str_field[0] in ('=', '+', '-', '@'):
        return "'" + str_field
    return str_field
