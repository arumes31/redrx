import unittest
from flask import Flask
from app.utils import is_safe_url, _is_domain_blocked

class TestPerfOptimization(unittest.TestCase):
    def test_is_domain_blocked(self):
        blocked = {"example.com", "malicious.net", "co.uk"}

        # Exact match
        self.assertTrue(_is_domain_blocked("example.com", blocked))
        # Subdomain match
        self.assertTrue(_is_domain_blocked("sub.example.com", blocked))
        self.assertTrue(_is_domain_blocked("a.b.c.example.com", blocked))
        # No match
        self.assertFalse(_is_domain_blocked("google.com", blocked))
        # Partial match (should be safe)
        self.assertFalse(_is_domain_blocked("notexample.com", blocked))
        # Parent domain match
        self.assertTrue(_is_domain_blocked("malicious.net", blocked))
        self.assertTrue(_is_domain_blocked("very.malicious.net", blocked))
        # Empty inputs
        self.assertFalse(_is_domain_blocked("", blocked))
        self.assertFalse(_is_domain_blocked("example.com", set()))

    def test_is_safe_url_integration(self):
        app = Flask(__name__)
        app.config['ENABLE_PHISHING_CHECK'] = True

        with app.app_context():
            blocked = {"example.com"}
            # This uses the refactored is_safe_url which calls _is_domain_blocked
            self.assertFalse(is_safe_url("http://example.com", blocked_domains_cache=blocked))
            self.assertFalse(is_safe_url("https://sub.example.com", blocked_domains_cache=blocked))
            self.assertTrue(is_safe_url("http://google.com", blocked_domains_cache=blocked))

if __name__ == "__main__":
    unittest.main()
