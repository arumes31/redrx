import pytest
from app.utils import is_safe_url

def test_is_safe_url_basic(app):
    with app.app_context():
        app.config['ENABLE_PHISHING_CHECK'] = True
        # Mocking blocked_domains_cache to avoid file system dependency
        blocked = {"blocked.com", "malicious.org", "phish.me.uk"}

        # Exact match
        assert is_safe_url("http://blocked.com", blocked) is False
        assert is_safe_url("https://blocked.com/path", blocked) is False

        # Subdomain match
        assert is_safe_url("http://sub.blocked.com", blocked) is False
        assert is_safe_url("http://very.deep.sub.malicious.org", blocked) is False

        # Suffix match (should not match if it's just a suffix of a label)
        assert is_safe_url("http://notblocked.com", blocked) is True

        # Safe domains
        assert is_safe_url("http://google.com", blocked) is True
        assert is_safe_url("https://github.com/jules", blocked) is True

        # Case insensitivity
        assert is_safe_url("http://BLOCKED.COM", blocked) is False
        assert is_safe_url("http://Sub.Malicious.Org", blocked) is False

def test_is_safe_url_edge_cases(app):
    with app.app_context():
        app.config['ENABLE_PHISHING_CHECK'] = True
        blocked = {"blocked.com"}

        # Invalid URLs
        assert is_safe_url(None, blocked) is False
        assert is_safe_url(123, blocked) is False
        assert is_safe_url("not a url", blocked) is False

        # Scheme check
        assert is_safe_url("ftp://blocked.com", blocked) is False
        assert is_safe_url("javascript:alert(1)", blocked) is False

        # Relative URL (netloc will be empty)
        assert is_safe_url("/path/to/something", blocked) is False

def test_is_safe_url_env_blocked(app, monkeypatch):
    with app.app_context():
        app.config['ENABLE_PHISHING_CHECK'] = True
        monkeypatch.setenv("BLOCKED_DOMAINS", "evil.com,bad.net")
        # Even if cache is empty, ENV should catch it
        assert is_safe_url("http://evil.com", set()) is False
        assert is_safe_url("http://sub.evil.com", set()) is False
        assert is_safe_url("http://bad.net", set()) is False
        assert is_safe_url("http://safe.com", set()) is True
