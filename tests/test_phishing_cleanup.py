import pytest
import os
from app.models import db, URL
from app.utils import cleanup_phishing_urls

@pytest.fixture
def phishing_setup(app):
    with app.app_context():
        # Create a temporary blocked domains file
        blocked_path = os.path.join(os.path.dirname(__file__), 'blocked_test.txt')
        with open(blocked_path, 'w') as f:
            f.write("phishing.com\n")
            f.write("evil.org\n")

        app.config['ENABLE_AUTO_REMOVE_PHISHING'] = True
        app.config['BLOCKED_DOMAINS_PATH'] = blocked_path

        yield blocked_path

        if os.path.exists(blocked_path):
            os.remove(blocked_path)

def test_cleanup_phishing_urls(app, phishing_setup):
    with app.app_context():
        # Clean up any existing URLs
        URL.query.delete()
        db.session.commit()

        # Add some URLs
        u1 = URL(short_code='safe', long_url='https://google.com')
        u2 = URL(short_code='bad1', long_url='https://phishing.com/login')
        u3 = URL(short_code='bad2', long_url='https://sub.evil.org/path')
        u4 = URL(short_code='safe2', long_url='https://example.com', rotate_targets=['https://phishing.com/x'])
        u5 = URL(short_code='safe3', long_url='https://example.com', rotate_targets=['https://safe.com'])

        db.session.add_all([u1, u2, u3, u4, u5])
        db.session.commit()

        assert URL.query.count() == 5

        cleanup_phishing_urls()

        remaining = URL.query.all()
        codes = [u.short_code for u in remaining]

        assert 'safe' in codes
        assert 'safe3' in codes
        assert 'bad1' not in codes
        assert 'bad2' not in codes
        assert 'safe2' not in codes # Should be removed because rotate_target is phishing
        assert len(remaining) == 2
