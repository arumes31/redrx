import os
import pytest
from app import create_app, db
from app.models import URL, User
from app.utils import cleanup_phishing_urls
from config import Config

class TestConfig(Config):
    TESTING = True
    SQLALCHEMY_DATABASE_URI = 'sqlite:///:memory:'
    WTF_CSRF_ENABLED = False
    ENABLE_AUTO_REMOVE_PHISHING = True
    BLOCKED_DOMAINS_PATH = 'test_blocked_domains.txt'

@pytest.fixture
def app():
    app = create_app(TestConfig)

    with app.app_context():
        yield app
        db.session.remove()
        db.drop_all()

    if os.path.exists('test_blocked_domains.txt'):
        os.remove('test_blocked_domains.txt')

def test_cleanup_phishing_urls(app):
    with app.app_context():
        # Create blocked domains file
        with open('test_blocked_domains.txt', 'w') as f:
            f.write('phishing.com\n')
            f.write('bad-actor.net\n')

        # Create URLs
        u1 = URL(long_url='https://safe.com/test', short_code='SAFE1')
        u2 = URL(long_url='https://phishing.com/steal', short_code='BAD1')
        u3 = URL(long_url='https://sub.phishing.com/steal', short_code='BAD2')
        u4 = URL(long_url='https://safe.com/rotate', short_code='ROT1', rotate_targets=['https://bad-actor.net/x'])
        u5 = URL(long_url='https://another-safe.com/', short_code='SAFE2')

        db.session.add_all([u1, u2, u3, u4, u5])
        db.session.commit()

        assert URL.query.count() == 5

        cleanup_phishing_urls()

        assert URL.query.count() == 2
        assert URL.query.filter_by(short_code='SAFE1').first() is not None
        assert URL.query.filter_by(short_code='SAFE2').first() is not None
        assert URL.query.filter_by(short_code='BAD1').first() is None
        assert URL.query.filter_by(short_code='BAD2').first() is None
        assert URL.query.filter_by(short_code='ROT1').first() is None
