import os
import unittest
from flask import Flask
from app.utils import is_safe_url

class TestUtilsSecurityCache(unittest.TestCase):
    def setUp(self):
        self.app = Flask(__name__)
        self.app.config['ENABLE_PHISHING_CHECK'] = False
        self.ctx = self.app.app_context()
        self.ctx.push()

    def tearDown(self):
        self.ctx.pop()

    def test_is_safe_url_env_caching(self):
        # 1. Set environment variable
        os.environ['BLOCKED_DOMAINS'] = 'malicious.com,evil.org'

        # 2. Check if blocked (should parse and cache)
        assert is_safe_url('http://malicious.com') is False
        assert is_safe_url('http://sub.malicious.com') is False
        assert is_safe_url('http://evil.org') is False
        assert is_safe_url('http://google.com') is True

        # 3. Change environment variable
        os.environ['BLOCKED_DOMAINS'] = 'blocked.com'

        # 4. Check if cache updated
        assert is_safe_url('http://blocked.com') is False
        assert is_safe_url('http://malicious.com') is True # No longer blocked

    def test_is_safe_url_config_fallback(self):
        # Clear env
        if 'BLOCKED_DOMAINS' in os.environ:
            del os.environ['BLOCKED_DOMAINS']

        self.app.config['BLOCKED_DOMAINS'] = ['configblocked.com']
        assert is_safe_url('http://configblocked.com') is False
        assert is_safe_url('http://google.com') is True

if __name__ == '__main__':
    unittest.main()
