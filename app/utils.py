import uuid
import qrcode
import io
import datetime
import base64
from PIL import Image

import requests
import geoip2.database
from flask import current_app
import time

import os
from urllib.parse import urlparse

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

        with open(path, 'w', encoding='utf-8') as f:
            for url in urls:
                url = url.strip()
                if not url:
                    continue
                try:
                    response = requests.get(url, timeout=10)
                    if response.status_code == 200:
                        f.write(response.text)
                        f.write('\n') # Ensure newline between lists
                except Exception:
                    continue
    except Exception: # nosec B110
        pass

def is_safe_url(target_url):
    """Checks if the URL is not in the blocked domains list."""
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

    path = current_app.config.get('BLOCKED_DOMAINS_PATH')
    if path and os.path.exists(path):
        try:
            with open(path, 'r', encoding='utf-8') as f:
                blocked_domains = {line.strip().lower() for line in f if line.strip()}
                
                # Check exact or parent domains
                parts = domain.split('.')
                for i in range(len(parts)):
                    check_domain = '.'.join(parts[i:])
                    if check_domain in blocked_domains:
                        return False
        except Exception: # nosec B110
            pass
            
    return True

def get_geo_info(ip):
    """Fetches country from IP using local MaxMind database."""
    if ip == '127.0.0.1' or ip.startswith('192.168.') or ip.startswith('10.'):
        return "Local Network"
    
    db_path = current_app.config.get('GEOIP_DB_PATH')
    if not db_path or not os.path.exists(db_path):
        return "Unknown (DB Missing)"

    try:
        with geoip2.database.Reader(db_path) as reader:
            response = reader.country(ip)
            return response.country.name or "Unknown"
    except Exception: # nosec B110
        pass
    return "Unknown"

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
