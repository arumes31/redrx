import os
import tempfile
from app.models import db, URL
from app.utils import cleanup_phishing_urls

def test_cleanup_phishing_urls(app):
    with app.app_context():
        # Setup blocked domains file
        fd, path = tempfile.mkstemp()
        try:
            with os.fdopen(fd, 'w') as tmp:
                tmp.write("phishing.com\nbad-actor.org\n")

            app.config['BLOCKED_DOMAINS_PATH'] = path
            app.config['ENABLE_AUTO_REMOVE_PHISHING'] = True

            # Create some URLs
            u1 = URL(short_code='safe', long_url='https://google.com')
            u2 = URL(short_code='bad', long_url='https://phishing.com/login')
            u3 = URL(short_code='rotate', long_url='https://safe.com')
            u3.rotate_targets = ["https://bad-actor.org/hack"]

            db.session.add_all([u1, u2, u3])
            db.session.commit()

            # Run cleanup
            cleanup_phishing_urls()

            # Verify
            remaining = URL.query.all()
            codes = [u.short_code for u in remaining]
            assert 'safe' in codes
            assert 'bad' not in codes
            assert 'rotate' not in codes
        finally:
            os.remove(path)
