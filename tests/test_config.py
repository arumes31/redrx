import sys
import importlib
import pytest

@pytest.fixture(autouse=True)
def clear_config_module():
    sys.modules.pop("config", None)
    yield
    sys.modules.pop("config", None)

def test_secret_key_from_env(monkeypatch):
    # Setup environment
    monkeypatch.setenv("SECRET_KEY", "test-secret-key")
    monkeypatch.setenv("FLASK_DEBUG", "false")

    import config
    importlib.reload(config)

    # Trigger validation
    config.Config.validate()

    assert config.Config.SECRET_KEY == "test-secret-key"

def test_secret_key_missing_production_raises_error(monkeypatch):
    # Setup environment: remove SECRET_KEY and ensure production mode
    monkeypatch.delenv("SECRET_KEY", raising=False)
    monkeypatch.setenv("FLASK_DEBUG", "false")

    import config
    importlib.reload(config)

    with pytest.raises(RuntimeError) as excinfo:
        config.Config.validate()
    assert "SECRET_KEY must be set in production" in str(excinfo.value)

def test_secret_key_dev_fallback(monkeypatch):
    # Setup environment: remove SECRET_KEY and ensure debug mode
    monkeypatch.delenv("SECRET_KEY", raising=False)
    monkeypatch.setenv("FLASK_DEBUG", "true")

    import config
    importlib.reload(config)

    # Trigger validation
    config.Config.validate()

    assert config.Config.SECRET_KEY == "dev-secret-key-do-not-use-in-production"

def test_config_import_safe_without_env(monkeypatch):
    # Ensure no SECRET_KEY or FLASK_DEBUG is set
    monkeypatch.delenv("SECRET_KEY", raising=False)
    monkeypatch.delenv("FLASK_DEBUG", raising=False)

    # Importing should NOT raise RuntimeError anymore
    import config
    importlib.reload(config)

    assert config.Config.SECRET_KEY is None
