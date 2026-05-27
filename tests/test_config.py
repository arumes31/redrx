import pytest
import importlib
import config


def test_config_no_secret_key_production(monkeypatch):
    # Ensure SECRET_KEY is not in env
    monkeypatch.delenv("SECRET_KEY", raising=False)
    # Ensure FLASK_DEBUG is false
    monkeypatch.setenv("FLASK_DEBUG", "false")

    with pytest.raises(RuntimeError) as excinfo:
        importlib.reload(config)
    assert (
        "SECRET_KEY environment variable is not set and the app is not in debug mode."
        in str(excinfo.value)
    )


def test_config_no_secret_key_debug(monkeypatch):
    # Ensure SECRET_KEY is not in env
    monkeypatch.delenv("SECRET_KEY", raising=False)
    # Ensure FLASK_DEBUG is true
    monkeypatch.setenv("FLASK_DEBUG", "true")

    importlib.reload(config)
    assert config.Config.SECRET_KEY == "dev-secret-key-do-not-use-in-production"


def test_config_with_secret_key_production(monkeypatch):
    # Set SECRET_KEY
    monkeypatch.setenv("SECRET_KEY", "test-secret-key")
    # Ensure FLASK_DEBUG is false
    monkeypatch.setenv("FLASK_DEBUG", "false")

    importlib.reload(config)
    assert config.Config.SECRET_KEY == "test-secret-key"


def test_config_with_secret_key_debug(monkeypatch):
    # Set SECRET_KEY
    monkeypatch.setenv("SECRET_KEY", "test-secret-key")
    # Ensure FLASK_DEBUG is true
    monkeypatch.setenv("FLASK_DEBUG", "true")

    importlib.reload(config)
    assert config.Config.SECRET_KEY == "test-secret-key"
