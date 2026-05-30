import sys
import importlib
import pytest
import os

@pytest.fixture(autouse=True)
def clear_config_module():
    # Save original environment
    old_env = os.environ.copy()
    # Set default safe environment for loading
    os.environ['FLASK_DEBUG'] = 'true'
    os.environ.pop('SECRET_KEY', None)

    sys.modules.pop('config', None)
    yield
    sys.modules.pop('config', None)

    # Restore original environment
    os.environ.clear()
    os.environ.update(old_env)

def test_secret_key_from_env(monkeypatch):
    # Setup environment
    monkeypatch.setenv('SECRET_KEY', 'test-secret-key')
    monkeypatch.setenv('FLASK_DEBUG', 'false')

    import config
    importlib.reload(config)

    assert config.Config.SECRET_KEY == 'test-secret-key'

def test_secret_key_missing_production_raises_error(monkeypatch):
    # Setup environment: remove SECRET_KEY and ensure production mode
    monkeypatch.delenv('SECRET_KEY', raising=False)
    monkeypatch.setenv('FLASK_DEBUG', 'false')

    with pytest.raises(RuntimeError) as excinfo:
        import config
        importlib.reload(config)
    assert "SECRET_KEY must be set in production" in str(excinfo.value)

def test_secret_key_dev_fallback(monkeypatch):
    # Setup environment: remove SECRET_KEY and ensure debug mode
    monkeypatch.delenv('SECRET_KEY', raising=False)
    monkeypatch.setenv('FLASK_DEBUG', 'true')

    import config
    importlib.reload(config)

    assert config.Config.SECRET_KEY == 'dev-secret-key-do-not-use-in-production'

def test_secret_key_empty_string_production_raises_error(monkeypatch):
    # Setup environment: set SECRET_KEY to empty string and ensure production mode
    monkeypatch.setenv('SECRET_KEY', '')
    monkeypatch.setenv('FLASK_DEBUG', 'false')

    with pytest.raises(RuntimeError) as excinfo:
        import config
        importlib.reload(config)
    assert "SECRET_KEY must be set in production" in str(excinfo.value)

def test_secret_key_whitespace_production_raises_error(monkeypatch):
    # Setup environment: set SECRET_KEY to whitespace and ensure production mode
    monkeypatch.setenv('SECRET_KEY', '   ')
    monkeypatch.setenv('FLASK_DEBUG', 'false')

    with pytest.raises(RuntimeError) as excinfo:
        import config
        importlib.reload(config)
    assert "SECRET_KEY must be set in production" in str(excinfo.value)

def test_secret_key_strips_whitespace(monkeypatch):
    # Setup environment: set SECRET_KEY with leading/trailing whitespace
    monkeypatch.setenv('SECRET_KEY', '  secret-with-spaces  ')
    monkeypatch.setenv('FLASK_DEBUG', 'false')

    import config
    importlib.reload(config)

    assert config.Config.SECRET_KEY == 'secret-with-spaces'
