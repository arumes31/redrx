import os
from app.utils import is_safe_url
import app.utils as utils
import pytest

def test_is_safe_url_blocked_domains(app):
    with app.app_context():
        # Reset cache for testing
        utils._blocked_env_domains = None
        os.environ['BLOCKED_DOMAINS'] = 'evil.com,bad.org'

        # Test exact match
        assert is_safe_url('http://evil.com') is False
        assert is_safe_url('https://bad.org') is False

        # Test subdomain
        assert is_safe_url('http://sub.evil.com') is False

        # Test non-blocked
        assert is_safe_url('http://google.com') is True

        # Test substring (should NOT be blocked)
        assert is_safe_url('http://not-evil.com') is True

def test_is_safe_url_caching(app):
    with app.app_context():
        # Reset cache
        utils._blocked_env_domains = None
        os.environ['BLOCKED_DOMAINS'] = 'initial.com'

        # First call loads cache
        assert is_safe_url('http://initial.com') is False

        # Change env
        os.environ['BLOCKED_DOMAINS'] = 'new.com'

        # Should still use cached 'initial.com'
        assert is_safe_url('http://initial.com') is False
        assert is_safe_url('http://new.com') is True
