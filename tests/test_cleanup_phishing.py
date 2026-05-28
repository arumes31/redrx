import pytest
from app.models import db, URL
from app.utils import cleanup_phishing_urls
import os

def test_cleanup_phishing_urls(app, tmp_path):
    with app.app_context():
        # Setup blocked domains file
        blocked_file = tmp_path / "blocked.txt"
        blocked_file.write_text("evil.com\nphish.net\n")

        app.config['ENABLE_AUTO_REMOVE_PHISHING'] = True
        app.config['BLOCKED_DOMAINS_PATH'] = str(blocked_file)

        # Create URLs
        u1 = URL(short_code='SAFE', long_url='https://google.com')
        u2 = URL(short_code='EVIL', long_url='https://evil.com/path')
        u3 = URL(short_code='SUBEVIL', long_url='https://sub.evil.com')
        u4 = URL(short_code='ROTEVIL', long_url='https://bing.com', rotate_targets=['https://phish.net/x'])
        u5 = URL(short_code='ROTSAFE', long_url='https://bing.com', rotate_targets=['https://safe.com'])

        db.session.add_all([u1, u2, u3, u4, u5])
        db.session.commit()

        assert URL.query.count() == 5

        cleanup_phishing_urls()

        remaining = [u.short_code for u in URL.query.all()]
        assert 'SAFE' in remaining
        assert 'ROTSAFE' in remaining
        assert 'EVIL' not in remaining
        assert 'SUBEVIL' not in remaining
        assert 'ROTEVIL' not in remaining
        assert len(remaining) == 2
