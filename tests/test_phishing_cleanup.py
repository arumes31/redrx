import os
import tempfile
import unittest.mock as mock
from sqlalchemy.exc import SQLAlchemyError
from app.models import db, URL
from app.utils import cleanup_phishing_urls
import logging

def test_cleanup_phishing_disabled(app):
    """Test that cleanup returns early if ENABLE_AUTO_REMOVE_PHISHING is False."""
    app.config['ENABLE_AUTO_REMOVE_PHISHING'] = False
    with mock.patch('app.utils.os.path.exists') as mock_exists:
        cleanup_phishing_urls()
        mock_exists.assert_not_called()

def test_cleanup_missing_path(app):
    """Test that cleanup returns early if BLOCKED_DOMAINS_PATH is not set or file missing."""
    app.config['ENABLE_AUTO_REMOVE_PHISHING'] = True
    app.config['BLOCKED_DOMAINS_PATH'] = None
    cleanup_phishing_urls() # Should not raise anything

    app.config['BLOCKED_DOMAINS_PATH'] = '/non/existent/path'
    cleanup_phishing_urls() # Should not raise anything

def test_cleanup_file_read_error(app, caplog):
    """Test handling of IOError/OSError when reading the phishing list."""
    app.config['ENABLE_AUTO_REMOVE_PHISHING'] = True
    caplog.set_level(logging.ERROR)
    with tempfile.NamedTemporaryFile() as tmp:
        app.config['BLOCKED_DOMAINS_PATH'] = tmp.name
        with mock.patch('app.utils.open', side_effect=IOError("Read error")):
            cleanup_phishing_urls()
            assert "Failed to read phishing list" in caplog.text

def test_cleanup_empty_blocked_domains(app):
    """Test that cleanup returns early if the phishing list is empty."""
    app.config['ENABLE_AUTO_REMOVE_PHISHING'] = True
    with tempfile.NamedTemporaryFile(mode='w', delete=False) as tmp:
        tmp.write("")
        tmp_path = tmp.name

    try:
        app.config['BLOCKED_DOMAINS_PATH'] = tmp_path
        with mock.patch('app.models.URL.query') as mock_query_class:
             # We need to mock the whole query chain or just .all()
             mock_all = mock_query_class.all
             cleanup_phishing_urls()
             mock_all.assert_not_called()
    finally:
        os.remove(tmp_path)

def test_cleanup_db_query_error(app, caplog):
    """Test handling of SQLAlchemyError during URL query."""
    app.config['ENABLE_AUTO_REMOVE_PHISHING'] = True
    caplog.set_level(logging.ERROR)
    with tempfile.NamedTemporaryFile(mode='w', delete=False) as tmp:
        tmp.write("phishing.com\n")
        tmp_path = tmp.name

    try:
        app.config['BLOCKED_DOMAINS_PATH'] = tmp_path
        with mock.patch('app.models.URL.query') as mock_query:
            mock_query.all.side_effect = SQLAlchemyError("DB error")
            cleanup_phishing_urls()
            assert "Failed to query URLs for phishing cleanup" in caplog.text
    finally:
        os.remove(tmp_path)

def test_cleanup_success(app):
    """Test successful identification and removal of phishing URLs."""
    app.config['ENABLE_AUTO_REMOVE_PHISHING'] = True
    with tempfile.NamedTemporaryFile(mode='w', delete=False) as tmp:
        tmp.write("phish.com\n")
        tmp.write("bad.org\n")
        tmp_path = tmp.name

    try:
        app.config['BLOCKED_DOMAINS_PATH'] = tmp_path
        with app.app_context():
            url1 = URL(short_code='SAFE1', long_url='https://google.com')
            url2 = URL(short_code='PHISH1', long_url='https://phish.com/login')
            url3 = URL(short_code='SUBPHISH', long_url='https://sub.bad.org/path')
            url4 = URL(short_code='ROTATEPHISH', long_url='https://safe.com')
            url4.rotate_targets = ['https://malware.com', 'https://phish.com/target']

            db.session.add_all([url1, url2, url3, url4])
            db.session.commit()

            cleanup_phishing_urls()

            remaining = URL.query.all()
            remaining_codes = [u.short_code for u in remaining]

            assert 'SAFE1' in remaining_codes
            assert 'PHISH1' not in remaining_codes
            assert 'SUBPHISH' not in remaining_codes
            assert 'ROTATEPHISH' not in remaining_codes
    finally:
        os.remove(tmp_path)

def test_cleanup_commit_error(app, caplog):
    """Test handling of SQLAlchemyError during commit and rollback."""
    app.config['ENABLE_AUTO_REMOVE_PHISHING'] = True
    caplog.set_level(logging.ERROR)
    with tempfile.NamedTemporaryFile(mode='w', delete=False) as tmp:
        tmp.write("phish.com\n")
        tmp_path = tmp.name

    try:
        app.config['BLOCKED_DOMAINS_PATH'] = tmp_path
        with app.app_context():
            url = URL(short_code='PHISH2', long_url='https://phish.com')
            db.session.add(url)
            db.session.commit()

            with mock.patch('app.models.db.session.commit', side_effect=SQLAlchemyError("Commit failed")):
                cleanup_phishing_urls()
                assert "Failed to commit phishing cleanup" in caplog.text

                # Verify it wasn't deleted due to rollback
                db.session.rollback() # Ensure session is clean
                assert URL.query.filter_by(short_code='PHISH2').first() is not None
    finally:
        os.remove(tmp_path)

def test_cleanup_processing_error(app, caplog):
    """Test that error in processing one URL doesn't stop the whole process."""
    app.config['ENABLE_AUTO_REMOVE_PHISHING'] = True
    caplog.set_level(logging.ERROR)
    with tempfile.NamedTemporaryFile(mode='w', delete=False) as tmp:
        tmp.write("phish.com\n")
        tmp_path = tmp.name

    try:
        app.config['BLOCKED_DOMAINS_PATH'] = tmp_path
        with app.app_context():
            url1 = URL(short_code='PHISH3', long_url='https://phish.com')
            url2 = URL(short_code='SAFE2', long_url='https://google.com')
            db.session.add_all([url1, url2])
            db.session.commit()

            # We need to trigger an exception during the loop.
            # urlparse is used in the loop.
            with mock.patch('app.utils.urlparse') as mock_urlparse:
                # First call (url1) raises, second (url2) succeeds
                # Actually urlparse is called once per URL.
                mock_urlparse.side_effect = [Exception("Processing error"), mock.DEFAULT]
                mock_urlparse.return_value = mock.Mock(netloc="google.com")

                cleanup_phishing_urls()
                assert "Error processing URL" in caplog.text

            # SAFE2 should still exist
            assert URL.query.filter_by(short_code='SAFE2').first() is not None
    finally:
        os.remove(tmp_path)

def test_cleanup_unexpected_outer_error(app, caplog):
    """Test handling of unexpected errors in the outer block."""
    app.config['ENABLE_AUTO_REMOVE_PHISHING'] = True
    caplog.set_level(logging.ERROR)
    with tempfile.NamedTemporaryFile(mode='w', delete=False) as tmp:
        tmp.write("phish.com\n")
        tmp_path = tmp.name

    try:
        app.config['BLOCKED_DOMAINS_PATH'] = tmp_path
        # Trigger unexpected error by mocking something fundamental
        with mock.patch('app.utils.open', side_effect=RuntimeError("Outer disaster")):
            cleanup_phishing_urls()
            assert "Unexpected error during phishing cleanup" in caplog.text
    finally:
        os.remove(tmp_path)
