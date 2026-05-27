import os
import tempfile
import pytest
from app.models import db, URL
from app.utils import cleanup_phishing_urls

def test_cleanup_phishing_urls(app):
    with app.app_context():
        # Create a temp blocked domains file
        fd, path = tempfile.mkstemp()
        try:
            with os.fdopen(fd, 'w') as f:
                f.write("phishing.com\n")
                f.write("malware.org\n")

            app.config['ENABLE_AUTO_REMOVE_PHISHING'] = True
            app.config['BLOCKED_DOMAINS_PATH'] = path

            # Create URLs
            u1 = URL(short_code='clean', long_url='https://google.com')
            u2 = URL(short_code='bad1', long_url='https://phishing.com/login')
            u3 = URL(short_code='bad2', long_url='https://sub.malware.org/path')
            u4 = URL(short_code='bad3', long_url='https://safe.com')
            u4.rotate_targets = ['https://phishing.com/bad']
            u5 = URL(short_code='clean2', long_url='https://safe.com')
            u5.rotate_targets = ['https://bing.com']

            db.session.add_all([u1, u2, u3, u4, u5])
            db.session.commit()

            assert URL.query.count() == 5

            cleanup_phishing_urls()

            remaining = URL.query.all()
            remaining_codes = [u.short_code for u in remaining]

            assert 'clean' in remaining_codes
            assert 'clean2' in remaining_codes
            assert 'bad1' not in remaining_codes
            assert 'bad2' not in remaining_codes
            assert 'bad3' not in remaining_codes
            assert len(remaining) == 2

        finally:
            if os.path.exists(path):
                os.remove(path)

def test_cleanup_phishing_urls_disabled(app):
    with app.app_context():
        app.config['ENABLE_AUTO_REMOVE_PHISHING'] = False

        u1 = URL(short_code='bad', long_url='https://phishing.com/login')
        db.session.add(u1)
        db.session.commit()

        cleanup_phishing_urls()

        assert URL.query.count() == 1
