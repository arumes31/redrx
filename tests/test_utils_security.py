from app.utils import is_safe_url

def test_is_safe_url_blocked_env(app):
    # Test that BLOCKED_DOMAINS from config is respected
    app.config['BLOCKED_DOMAINS'] = ['malicious.com', 'evil.org']

    with app.app_context():
        assert is_safe_url('http://malicious.com') is False
        assert is_safe_url('http://sub.malicious.com') is False
        assert is_safe_url('http://evil.org/path') is False
        assert is_safe_url('http://goodmalicious.com') is True
        assert is_safe_url('http://google.com') is True

def test_is_safe_url_with_port(app):
    app.config['BLOCKED_DOMAINS'] = ['malicious.com']

    with app.app_context():
        # Verify that URLs with ports are still blocked (regression test)
        assert is_safe_url('http://malicious.com:8080') is False
        assert is_safe_url('http://sub.malicious.com:443') is False

def test_is_safe_url_case_insensitivity(app):
    # Since config.py handles lowercasing, we test the effect
    app.config['BLOCKED_DOMAINS'] = ['malicious.com']

    with app.app_context():
        assert is_safe_url('http://malicious.com') is False
        assert is_safe_url('http://MALICIOUS.COM') is False

def test_is_safe_url_empty_env(app):
    app.config['BLOCKED_DOMAINS'] = []

    with app.app_context():
        assert is_safe_url('http://anything.com') is True
