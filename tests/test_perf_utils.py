import pytest
from app.utils import is_safe_url
from flask import current_app
import os

def test_is_safe_url_blocked_domains(app):
    # Mock config
    app.config['ENABLE_PHISHING_CHECK'] = True

    blocked_domains = {'example.com', 'malicious.net', 'bad.co.uk'}

    # Test subdomains
    assert is_safe_url('http://example.com', blocked_domains_cache=blocked_domains) is False
    assert is_safe_url('https://sub.example.com', blocked_domains_cache=blocked_domains) is False
    assert is_safe_url('http://very.deep.sub.example.com', blocked_domains_cache=blocked_domains) is False

    assert is_safe_url('https://malicious.net', blocked_domains_cache=blocked_domains) is False
    assert is_safe_url('http://test.malicious.net', blocked_domains_cache=blocked_domains) is False

    assert is_safe_url('http://bad.co.uk', blocked_domains_cache=blocked_domains) is False
    assert is_safe_url('http://more.bad.co.uk', blocked_domains_cache=blocked_domains) is False

    # Test safe domains
    assert is_safe_url('https://google.com', blocked_domains_cache=blocked_domains) is True
    assert is_safe_url('http://example.net', blocked_domains_cache=blocked_domains) is True
    assert is_safe_url('https://co.uk', blocked_domains_cache=blocked_domains) is True

def test_is_safe_url_no_phishing_check(app):
    app.config['ENABLE_PHISHING_CHECK'] = False
    blocked_domains = {'example.com'}
    assert is_safe_url('http://example.com', blocked_domains_cache=blocked_domains) is True

def test_is_safe_url_env_blocked(app):
    os.environ['BLOCKED_DOMAINS'] = 'evil.com,bad.org'
    assert is_safe_url('http://evil.com/path') is False
    assert is_safe_url('https://sub.evil.com') is False
    assert is_safe_url('http://bad.org') is False
    assert is_safe_url('http://google.com') is True
    del os.environ['BLOCKED_DOMAINS']
