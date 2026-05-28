import os
import pytest
from app.utils import is_safe_url

def test_is_safe_url_blocked_env(app, monkeypatch):
    # Test that BLOCKED_DOMAINS from environment is respected
    monkeypatch.setenv('BLOCKED_DOMAINS', 'malicious.com,evil.org')

    with app.app_context():
        # Current implementation uses substring matching
        assert is_safe_url('http://malicious.com') is False
        assert is_safe_url('http://sub.malicious.com') is False
        assert is_safe_url('http://evil.org/path') is False
        assert is_safe_url('http://google.com') is True

def test_is_safe_url_with_port(app, monkeypatch):
    monkeypatch.setenv('BLOCKED_DOMAINS', 'malicious.com')

    with app.app_context():
        # Verify that URLs with ports are still blocked (regression test)
        assert is_safe_url('http://malicious.com:8080') is False
        assert is_safe_url('http://sub.malicious.com:443') is False

def test_is_safe_url_case_insensitivity(app, monkeypatch):
    monkeypatch.setenv('BLOCKED_DOMAINS', 'MALICIOUS.com')

    with app.app_context():
        assert is_safe_url('http://malicious.com') is False
        assert is_safe_url('http://MALICIOUS.COM') is False

def test_is_safe_url_empty_env(app, monkeypatch):
    monkeypatch.setenv('BLOCKED_DOMAINS', '')

    with app.app_context():
        assert is_safe_url('http://anything.com') is True

def test_is_safe_url_dynamic_update(app, monkeypatch):
    # Test that the cache correctly updates when the environment variable changes
    monkeypatch.setenv('BLOCKED_DOMAINS', 'first.com')
    with app.app_context():
        assert is_safe_url('http://first.com') is False
        assert is_safe_url('http://second.com') is True

    monkeypatch.setenv('BLOCKED_DOMAINS', 'second.com')
    with app.app_context():
        assert is_safe_url('http://first.com') is True
        assert is_safe_url('http://second.com') is False
