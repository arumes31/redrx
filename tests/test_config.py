import pytest
import os
from unittest.mock import patch
from config import Config

def test_config_production_missing_secret_key():
    # Simulate production environment without SECRET_KEY
    env_vars = {
        'FLASK_DEBUG': 'false',
        'SECRET_KEY': ''
    }
    with patch.dict(os.environ, env_vars, clear=True):
        # Reloading Config might be tricky if it's already imported,
        # but let's see if we can just re-instantiate or if the logic runs on import.
        # The logic is currently at the class level, so it runs on import/definition.
        # We might need to use importlib to reload the module.
        import importlib
        import config
        with pytest.raises(RuntimeError) as excinfo:
            importlib.reload(config)
        assert "SECRET_KEY environment variable is required in production" in str(excinfo.value)

def test_config_production_with_secret_key():
    # Simulate production environment with SECRET_KEY
    env_vars = {
        'FLASK_DEBUG': 'false',
        'SECRET_KEY': 'super-secret-production-key'
    }
    with patch.dict(os.environ, env_vars, clear=True):
        import importlib
        import config
        reloaded_config = importlib.reload(config)
        assert reloaded_config.Config.SECRET_KEY == 'super-secret-production-key'

def test_config_development_missing_secret_key():
    # Simulate development environment without SECRET_KEY
    env_vars = {
        'FLASK_DEBUG': 'true',
        'SECRET_KEY': ''
    }
    with patch.dict(os.environ, env_vars, clear=True):
        import importlib
        import config
        reloaded_config = importlib.reload(config)
        assert reloaded_config.Config.SECRET_KEY == 'dev-secret-key-do-not-use-in-production'
        assert reloaded_config.Config.DEBUG is True

def test_config_development_with_secret_key():
    # Simulate development environment with SECRET_KEY
    env_vars = {
        'FLASK_DEBUG': 'true',
        'SECRET_KEY': 'dev-override-key'
    }
    with patch.dict(os.environ, env_vars, clear=True):
        import importlib
        import config
        reloaded_config = importlib.reload(config)
        assert reloaded_config.Config.SECRET_KEY == 'dev-override-key'
