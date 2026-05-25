import uuid
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

        urls = URL.query.all()
        removed_count = 0
        
        for url_entry in urls:
            # Check main long_url
            try:
                domain = urlparse(url_entry.long_url).netloc.lower()
                is_phishing = False
                if domain:
                    parts = domain.split('.')
                    for i in range(len(parts)):
                        if '.'.join(parts[i:]) in blocked_domains:
                            is_phishing = True
                            break
                
                # Check rotate_targets if main is clean
                if not is_phishing and url_entry.rotate_targets:
                    for target in url_entry.rotate_targets:
                        target_domain = urlparse(target).netloc.lower()
                        if target_domain:
                            parts = target_domain.split('.')
                            for i in range(len(parts)):
                                if '.'.join(parts[i:]) in blocked_domains:
                                    is_phishing = True
                                    break
                        if is_phishing: break

                if is_phishing:
                    db.session.delete(url_entry)
                    removed_count += 1
            except Exception: # nosec B112
                continue
        
        if removed_count > 0:
            db.session.commit()
            
    except Exception: # nosec B110
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

    # 1. Check manual overrides from ENV
    blocked_env = os.environ.get('BLOCKED_DOMAINS', '').split(',')
    domain = ""
    try:
        domain = urlparse(target_url).netloc.lower()
        if not domain: # For relative or malformed URLs
             return False
             
        for b in blocked_env:
            if b.strip() and b.strip().lower() in domain:
                return False
    except Exception:
        return False

    # 2. Check downloaded list
    if not current_app.config.get('ENABLE_PHISHING_CHECK'):
        return True

    blocked_domains = blocked_domains_cache if blocked_domains_cache is not None else get_blocked_domains()
    if blocked_domains:
        parts = domain.split('.')
        for i in range(len(parts)):
            check_domain = '.'.join(parts[i:])
            if check_domain in blocked_domains:
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

def get_geo_info(ip, request=None):
    """Fetches country from IP using local MaxMind database or Cloudflare header with Redis cache."""
    global _redis_client
    if _redis_client is None:
        _redis_client = _get_redis_client()

    # 1. Check Redis Cache
    if _redis_client:
        try:
            cached_val = _redis_client.get(f"{_GEO_PREFIX}{ip}")
            if cached_val:
                return cached_val
        except Exception:
            pass # Fallback to lookup on Redis error

    # 2. Check Cloudflare
    if request:
        cf_country = get_client_country(request)
        if cf_country:
            # Update Cache if Redis is available
            if _redis_client:
                try:
                    _redis_client.setex(f"{_GEO_PREFIX}{ip}", _GEO_CACHE_TTL, cf_country)
                except Exception:
                    pass
            return cf_country

    if ip == '127.0.0.1' or ip.startswith('192.168.') or ip.startswith('10.') or ip.startswith('172.'):
        return "Local Network"
    
    # 3. Check Local DB
    db_path = current_app.config.get('GEOIP_DB_PATH')
    if not db_path or not os.path.exists(db_path):
        return "Unknown (DB Missing)"

    country = "Unknown"
    try:
        with geoip2.database.Reader(db_path) as reader:
            response = reader.country(ip)
            country = response.country.name or "Unknown"
    except Exception: # nosec B110
        pass
    
    # 4. Update Redis Cache
    if _redis_client:
        try:
            _redis_client.setex(f"{_GEO_PREFIX}{ip}", _GEO_CACHE_TTL, country)
        except Exception:
            pass

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
    # Using microsecond for more "random" feel on rapid refreshes
    idx = hash(str(datetime.datetime.now().microsecond)) % len(rotate_targets)
    return rotate_targets[idx]

def get_qr_data_url(data, color='black', bg='white', logo_img=None):
    """Returns a base64 encoded data URL for the QR code."""
    img_buffer = generate_qr(data, color, bg, logo_img)
    return base64.b64encode(img_buffer.read()).decode()
