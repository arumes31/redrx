import os
import importlib
import pytest

def test_secret_key_from_env():
    # Setup environment
    os.environ['SECRET_KEY'] = 'test-secret-key'
    os.environ['FLASK_DEBUG'] = 'false'

    import config
    importlib.reload(config)

    assert config.Config.SECRET_KEY == 'test-secret-key'

def test_secret_key_missing_production_raises_error():
    # Setup environment: remove SECRET_KEY and ensure production mode
    if 'SECRET_KEY' in os.environ:
        del os.environ['SECRET_KEY']
    os.environ['FLASK_DEBUG'] = 'false'

    import config
    with pytest.raises(RuntimeError) as excinfo:
        importlib.reload(config)
    assert "SECRET_KEY must be set in production" in str(excinfo.value)

def test_secret_key_dev_fallback():
    # Setup environment: remove SECRET_KEY and ensure debug mode
    if 'SECRET_KEY' in os.environ:
        del os.environ['SECRET_KEY']
    os.environ['FLASK_DEBUG'] = 'true'

    import config
    importlib.reload(config)

    assert config.Config.SECRET_KEY == 'dev-secret-key-do-not-use-in-production'
