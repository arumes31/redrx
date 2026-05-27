import pytest
import os
import tempfile
from unittest.mock import patch, MagicMock
from app.models import db, URL
from app.utils import cleanup_phishing_urls

def test_cleanup_phishing_urls_success(app):
    # Setup test data
    with app.app_context():
        u1 = URL(short_code='CLEAN', long_url='https://google.com')
        u2 = URL(short_code='PHISH', long_url='https://phishing.com/login')
        u3 = URL(short_code='PHISHSUB', long_url='https://sub.evil.com/path')
        db.session.add_all([u1, u2, u3])
        db.session.commit()

        # Create a temp blocked domains file
        fd, path = tempfile.mkstemp()
        try:
            with os.fdopen(fd, 'w') as f:
                f.write('phishing.com\n')
                f.write('evil.com\n')

            app.config['ENABLE_AUTO_REMOVE_PHISHING'] = True
            app.config['BLOCKED_DOMAINS_PATH'] = path

            cleanup_phishing_urls()

            # Verify PHISH and PHISHSUB are removed, CLEAN remains
            remaining = URL.query.all()
            remaining_codes = [u.short_code for u in remaining]
            assert 'CLEAN' in remaining_codes
            assert 'PHISH' not in remaining_codes
            assert 'PHISHSUB' not in remaining_codes
        finally:
            os.remove(path)

def test_cleanup_phishing_urls_rotate_targets(app):
    with app.app_context():
        u1 = URL(short_code='ROTATE', long_url='https://google.com', rotate_targets=['https://safe.com', 'https://malware.org'])
        db.session.add(u1)
        db.session.commit()

        fd, path = tempfile.mkstemp()
        try:
            with os.fdopen(fd, 'w') as f:
                f.write('malware.org\n')

            app.config['ENABLE_AUTO_REMOVE_PHISHING'] = True
            app.config['BLOCKED_DOMAINS_PATH'] = path

            cleanup_phishing_urls()

            assert URL.query.filter_by(short_code='ROTATE').first() is None
        finally:
            os.remove(path)

def test_cleanup_phishing_urls_file_error(app):
    with app.app_context():
        app.config['ENABLE_AUTO_REMOVE_PHISHING'] = True
        app.config['BLOCKED_DOMAINS_PATH'] = 'non_existent_file.txt'

        # We need to mock os.path.exists to return True so it tries to open it
        with patch('os.path.exists', return_value=True):
            with patch('builtins.open', side_effect=OSError("Read error")):
                with patch.object(app.logger, 'error') as mock_log:
                    cleanup_phishing_urls()
                    mock_log.assert_called_once()
                    assert "File error" in mock_log.call_args[0][0]

def test_cleanup_phishing_urls_db_error(app):
    with app.app_context():
        # Setup test data
        u1 = URL(short_code='PHISH', long_url='https://phishing.com/login')
        db.session.add(u1)
        db.session.commit()

        fd, path = tempfile.mkstemp()
        try:
            with os.fdopen(fd, 'w') as f:
                f.write('phishing.com\n')

            app.config['ENABLE_AUTO_REMOVE_PHISHING'] = True
            app.config['BLOCKED_DOMAINS_PATH'] = path

            from sqlalchemy.exc import SQLAlchemyError
            with patch('app.models.db.session.commit', side_effect=SQLAlchemyError("DB fail")):
                with patch('app.models.db.session.rollback') as mock_rollback:
                    with patch.object(app.logger, 'error') as mock_log:
                        cleanup_phishing_urls()
                        mock_rollback.assert_called_once()
                        mock_log.assert_called_once()
                        assert "Database error" in mock_log.call_args[0][0]
        finally:
            os.remove(path)

def test_cleanup_phishing_urls_unexpected_error(app):
    with app.app_context():
        app.config['ENABLE_AUTO_REMOVE_PHISHING'] = True
        app.config['BLOCKED_DOMAINS_PATH'] = 'some_path'

        with patch('os.path.exists', return_value=True):
            # Raise an unexpected exception during file read
            with patch('builtins.open', side_effect=Exception("Boom")):
                with patch.object(app.logger, 'error') as mock_log:
                    cleanup_phishing_urls()
                    mock_log.assert_called_once()
                    assert "Unexpected error" in mock_log.call_args[0][0]
