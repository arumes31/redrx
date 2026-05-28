from app.models import db, URL
from app.utils import cleanup_phishing_urls

def test_cleanup_phishing_urls(app, tmp_path):
    # Setup blocked domains file
    blocked_file = tmp_path / "blocked.txt"
    blocked_file.write_text("phishing.com\nmalware.org")

    app.config['ENABLE_AUTO_REMOVE_PHISHING'] = True
    app.config['BLOCKED_DOMAINS_PATH'] = str(blocked_file)

    with app.app_context():
        # Clean URL
        url1 = URL(short_code='SAFE', long_url='https://google.com')
        # Phishing main URL
        url2 = URL(short_code='PHISH', long_url='https://phishing.com/login')
        # Phishing subdomain
        url3 = URL(short_code='SUBPHISH', long_url='https://sub.phishing.com/login')
        # Phishing rotate target
        url4 = URL(short_code='ROTATE', long_url='https://safe.com', rotate_targets=['https://malware.org/bad'])
        # Clean rotate target
        url5 = URL(short_code='SAFEROT', long_url='https://safe.com', rotate_targets=['https://ok.com'])

        db.session.add_all([url1, url2, url3, url4, url5])
        db.session.commit()

        cleanup_phishing_urls()

        remaining = URL.query.all()
        remaining_codes = {u.short_code for u in remaining}

        assert 'SAFE' in remaining_codes
        assert 'PHISH' not in remaining_codes
        assert 'SUBPHISH' not in remaining_codes
        assert 'ROTATE' not in remaining_codes
        assert 'SAFEROT' in remaining_codes
        assert len(remaining) == 2

def test_cleanup_phishing_urls_disabled(app, tmp_path):
    blocked_file = tmp_path / "blocked.txt"
    blocked_file.write_text("phishing.com")

    app.config['ENABLE_AUTO_REMOVE_PHISHING'] = False
    app.config['BLOCKED_DOMAINS_PATH'] = str(blocked_file)

    with app.app_context():
        url1 = URL(short_code='PHISH', long_url='https://phishing.com/login')
        db.session.add(url1)
        db.session.commit()

        cleanup_phishing_urls()

        assert URL.query.filter_by(short_code='PHISH').first() is not None
