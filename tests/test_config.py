import os
import importlib
import pytest

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

    import config
    with pytest.raises(RuntimeError) as excinfo:
        importlib.reload(config)
    assert "SECRET_KEY must be set in production" in str(excinfo.value)

def test_secret_key_dev_fallback(monkeypatch):
    # Setup environment: remove SECRET_KEY and ensure debug mode
    monkeypatch.delenv('SECRET_KEY', raising=False)
    monkeypatch.setenv('FLASK_DEBUG', 'true')

    import config
    importlib.reload(config)

    assert config.Config.SECRET_KEY == 'dev-secret-key-do-not-use-in-production'
