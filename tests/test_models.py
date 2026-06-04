import json
from datetime import datetime, timedelta, timezone
from app.models import db, User, URL, Click

def test_user_creation(app):
    with app.app_context():
        user = User(
            username='newuser',
            email='new@example.com',
            password_hash='hash'
        )
        db.session.add(user)
        db.session.commit()

        saved_user = User.query.filter_by(username='newuser').first()
        assert saved_user is not None
        assert saved_user.email == 'new@example.com'
        assert saved_user.created_at is not None

def test_url_rotate_targets_list(app):
    with app.app_context():
        targets = ['https://site1.com', 'https://site2.com']
        url = URL(short_code='rotate_list', long_url='https://main.com')
        url.rotate_targets = targets
        db.session.add(url)
        db.session.commit()

        saved_url = URL.query.filter_by(short_code='rotate_list').first()
        assert saved_url.rotate_targets == targets
        assert json.loads(saved_url._rotate_targets) == targets

def test_url_rotate_targets_json_string(app):
    with app.app_context():
        targets = ['https://site1.com', 'https://site2.com']
        json_targets = json.dumps(targets)
        url = URL(short_code='rotate_json', long_url='https://main.com')
        url.rotate_targets = json_targets
        db.session.add(url)
        db.session.commit()

        saved_url = URL.query.filter_by(short_code='rotate_json').first()
        assert saved_url.rotate_targets == targets
        assert json.loads(saved_url._rotate_targets) == targets

def test_url_rotate_targets_single_string(app):
    with app.app_context():
        target = 'https://site1.com'
        url = URL(short_code='rotate_single', long_url='https://main.com')
        url.rotate_targets = target
        db.session.add(url)
        db.session.commit()

        saved_url = URL.query.filter_by(short_code='rotate_single').first()
        assert saved_url.rotate_targets == [target]
        assert json.loads(saved_url._rotate_targets) == [target]

def test_url_rotate_targets_none(app):
    with app.app_context():
        url = URL(short_code='rotate_none', long_url='https://main.com')
        url.rotate_targets = None
        db.session.add(url)
        db.session.commit()

        saved_url = URL.query.filter_by(short_code='rotate_none').first()
        assert saved_url.rotate_targets == []
        assert saved_url._rotate_targets is None

def test_url_rotate_targets_empty_list(app):
    with app.app_context():
        url = URL(short_code='rotate_empty', long_url='https://main.com')
        url.rotate_targets = []
        db.session.add(url)
        db.session.commit()

        saved_url = URL.query.filter_by(short_code='rotate_empty').first()
        assert saved_url.rotate_targets == []
        assert saved_url._rotate_targets is None

def test_url_rotate_targets_tuple(app):
    with app.app_context():
        targets = ('https://site1.com', 'https://site2.com')
        url = URL(short_code='rotate_tuple', long_url='https://main.com')
        url.rotate_targets = targets
        db.session.add(url)
        db.session.commit()

        saved_url = URL.query.filter_by(short_code='rotate_tuple').first()
        assert saved_url.rotate_targets == list(targets)
        assert json.loads(saved_url._rotate_targets) == list(targets)

def test_url_rotate_targets_non_list_json(app):
    with app.app_context():
        json_str = '{"a": 1}'
        url = URL(short_code='rotate_bad_json', long_url='https://main.com')
        url.rotate_targets = json_str
        db.session.add(url)
        db.session.commit()

        saved_url = URL.query.filter_by(short_code='rotate_bad_json').first()
        # Should be treated as a single string and wrapped in a list
        assert saved_url.rotate_targets == [json_str]

def test_url_is_active(app):
    with app.app_context():
        now = datetime.now(timezone.utc).replace(tzinfo=None)

        url = URL(short_code='active1', long_url='https://a.com', is_enabled=True)
        assert url.is_active() is True

        url.is_enabled = False
        assert url.is_active() is False
        url.is_enabled = True

        url.start_at = now + timedelta(hours=1)
        assert url.is_active() is False

        url.start_at = now - timedelta(hours=1)
        assert url.is_active() is True

        url.end_at = now - timedelta(minutes=1)
        assert url.is_active() is False

        url.end_at = now + timedelta(hours=1)
        assert url.is_active() is True

        url.expires_at = now - timedelta(seconds=1)
        assert url.is_active() is False

        url.expires_at = now + timedelta(hours=1)
        assert url.is_active() is True

def test_click_and_relationship(app):
    with app.app_context():
        url = URL(short_code='clicky', long_url='https://google.com')
        db.session.add(url)
        db.session.commit()

        click = Click(
            url_id=url.id,
            ip_address='127.0.0.1',
            country='Local',
            browser='Chrome',
            platform='MacOS',
            referrer='https://twitter.com'
        )
        db.session.add(click)
        db.session.commit()

        assert len(url.clicks) == 1
        assert url.clicks[0].ip_address == '127.0.0.1'
        assert url.clicks[0].url.short_code == 'clicky'

def test_url_cascade_delete(app):
    with app.app_context():
        url = URL(short_code='cascade', long_url='https://google.com')
        db.session.add(url)
        db.session.commit()

        click = Click(url_id=url.id)
        db.session.add(click)
        db.session.commit()

        assert Click.query.filter_by(url_id=url.id).count() == 1

        db.session.delete(url)
        db.session.commit()

        assert Click.query.filter_by(url_id=url.id).count() == 0

def test_url_rotate_targets_other_type(app):
    with app.app_context():
        # Test with an integer (not list, tuple, or string)
        target = 123
        url = URL(short_code='rotate_int', long_url='https://main.com')
        url.rotate_targets = target
        db.session.add(url)
        db.session.commit()

        saved_url = URL.query.filter_by(short_code='rotate_int').first()
        assert saved_url.rotate_targets == [str(target)]
