import pytest
import os
import tempfile
from unittest.mock import patch, MagicMock
from app.models import db, URL, User
from app.utils import cleanup_phishing_urls
from flask import current_app
from sqlalchemy.exc import SQLAlchemyError

def test_cleanup_phishing_urls_enabled_check(app):
    """Test that it returns early if auto-remove is disabled."""
    app.config['ENABLE_AUTO_REMOVE_PHISHING'] = False
    with patch('app.utils.os.path.exists') as mock_exists:
        cleanup_phishing_urls()
        mock_exists.assert_not_called()

def test_cleanup_phishing_urls_no_path(app):
    """Test that it returns early if no path is configured or file doesn't exist."""
    app.config['ENABLE_AUTO_REMOVE_PHISHING'] = True
    app.config['BLOCKED_DOMAINS_PATH'] = None
    cleanup_phishing_urls()
    # Should not raise any error

def test_cleanup_phishing_urls_removes_malicious(app):
    """Test that it correctly removes phishing URLs."""
    app.config['ENABLE_AUTO_REMOVE_PHISHING'] = True

    with tempfile.NamedTemporaryFile(mode='w', delete=False) as tmp:
        tmp.write("malicious.com\n")
        tmp_path = tmp.name

    app.config['BLOCKED_DOMAINS_PATH'] = tmp_path

    try:
        with app.app_context():
            user = User(username='u1', email='u1@e.com', password_hash='h')
            db.session.add(user)
            db.session.commit()
            user_id = user.id

            u1 = URL(short_code='safe', long_url='https://google.com', user_id=user_id)
            u2 = URL(short_code='bad', long_url='https://malicious.com/phish', user_id=user_id)
            u3 = URL(short_code='sub', long_url='https://sub.malicious.com', user_id=user_id)
            db.session.add_all([u1, u2, u3])
            db.session.commit()

            cleanup_phishing_urls()

            remaining = URL.query.all()
            codes = [u.short_code for u in remaining]
            assert 'safe' in codes
            assert 'bad' not in codes
            assert 'sub' not in codes
    finally:
        if os.path.exists(tmp_path):
            os.remove(tmp_path)

def test_cleanup_phishing_urls_file_error(app):
    """Test handling of file read errors."""
    app.config['ENABLE_AUTO_REMOVE_PHISHING'] = True
    app.config['BLOCKED_DOMAINS_PATH'] = '/nonexistent/path/phish.txt'

    with patch('app.utils.os.path.exists', return_value=True):
        with patch('app.utils.open', side_effect=IOError("Permission denied")):
            with patch.object(current_app.logger, 'error') as mock_log:
                cleanup_phishing_urls()
                mock_log.assert_called_with("Failed to read blocked domains file: Permission denied")

def test_cleanup_phishing_urls_commit_error(app):
    """Test handling of database commit errors."""
    app.config['ENABLE_AUTO_REMOVE_PHISHING'] = True

    with tempfile.NamedTemporaryFile(mode='w', delete=False) as tmp:
        tmp.write("bad.com\n")
        tmp_path = tmp.name
    app.config['BLOCKED_DOMAINS_PATH'] = tmp_path

    try:
        with app.app_context():
            user = User(username='u_err', email='u_err@e.com', password_hash='h')
            db.session.add(user)
            db.session.commit()

            u1 = URL(short_code='bad', long_url='https://bad.com', user_id=user.id)
            db.session.add(u1)
            db.session.commit()

            with patch('app.models.db.session.commit', side_effect=SQLAlchemyError("DB Fail")):
                with patch('app.models.db.session.rollback') as mock_rollback:
                    with patch.object(current_app.logger, 'error') as mock_log:
                        cleanup_phishing_urls()
                        mock_rollback.assert_called()
                        # The error might be logged twice (inner commit fail, outer catch-all)
                        # but we just check if it was logged with the right message
                        mock_log.assert_any_call("Failed to commit phishing removal: DB Fail")
    finally:
        if os.path.exists(tmp_path):
            os.remove(tmp_path)

def test_cleanup_phishing_urls_processing_error(app):
    """Test handling of errors during URL processing."""
    app.config['ENABLE_AUTO_REMOVE_PHISHING'] = True

    with tempfile.NamedTemporaryFile(mode='w', delete=False) as tmp:
        tmp.write("bad.com\n")
        tmp_path = tmp.name
    app.config['BLOCKED_DOMAINS_PATH'] = tmp_path

    try:
        with app.app_context():
            user = User(username='u_proc', email='u_proc@e.com', password_hash='h')
            db.session.add(user)
            db.session.commit()

            u1 = URL(short_code='broken', long_url='https://bad.com', user_id=user.id)
            db.session.add(u1)
            db.session.commit()

            # Mock urlparse to raise an exception for this URL
            with patch('app.utils.urlparse', side_effect=Exception("Parse error")):
                with patch.object(current_app.logger, 'error') as mock_log:
                    cleanup_phishing_urls()
                    mock_log.assert_any_call(f"Error checking URL {u1.id} for phishing: Parse error")
    finally:
        if os.path.exists(tmp_path):
            os.remove(tmp_path)
