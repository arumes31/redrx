import os
from flask import current_app
from urllib.parse import urlparse

def _get_blocked_domains_list(path):
    """Loads blocked domains from a file."""
    if not path or not os.path.exists(path):
        return set()
    try:
        with open(path, 'r', encoding='utf-8') as f:
            return {line.strip().lower() for line in f if line.strip()}
    except Exception:
        return set()

def _cleanup_urls_batch(db, URL, blocked_domains):
    """Processes URL entries and deletes those identified as phishing."""
    removed_count = 0
    urls = URL.query.yield_per(100)
    for url_entry in urls:
        try:
            if _is_url_entry_phishing(url_entry, blocked_domains):
                db.session.delete(url_entry)
                removed_count += 1
            else:
                db.session.expunge(url_entry)
        except Exception: # nosec B112
            continue
    return removed_count

def cleanup_phishing_urls():
    """Removes URLs from database that are found in the phishing lists."""
    if not current_app.config.get('ENABLE_AUTO_REMOVE_PHISHING'):
        return

    path = current_app.config.get('BLOCKED_DOMAINS_PATH')
    blocked_domains = _get_blocked_domains_list(path)
    if not blocked_domains:
        return

    from app.models import db, URL
    try:
        removed_count = _cleanup_urls_batch(db, URL, blocked_domains)
        if removed_count > 0:
            db.session.commit()
    except Exception: # nosec B110
        try:
            db.session.rollback()
        except Exception:
            pass
