from datetime import datetime, timedelta, timezone
import json
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

def test_url_rotate_targets_polymorphic(app):
    with app.app_context():
        url = URL(short_code='rotate', long_url='https://main.com')

        # 1. Test setting list
        targets = ['https://site1.com', 'https://site2.com']
        url.rotate_targets = targets
        assert url.rotate_targets == targets
        assert json.loads(url._rotate_targets) == targets

        # 2. Test setting JSON string
        json_targets = json.dumps(['https://json1.com', 'https://json2.com'])
        url.rotate_targets = json_targets
        assert url.rotate_targets == ['https://json1.com', 'https://json2.com']

        # 3. Test setting plain string (single URL)
        single_url = 'https://single.com'
        url.rotate_targets = single_url
        assert url.rotate_targets == [single_url]

        # 4. Test setting None
        url.rotate_targets = None
        assert url.rotate_targets == []
        assert url._rotate_targets is None

        # 5. Test setting empty list
        url.rotate_targets = []
        assert url.rotate_targets == []
        assert url._rotate_targets is None

        # 6. Test setting empty string
        url.rotate_targets = ""
        assert url.rotate_targets == []
        assert url._rotate_targets is None

def test_url_is_active(app):
    with app.app_context():
        now = datetime.now(timezone.utc).replace(tzinfo=None)

        url = URL(short_code='active1', long_url='https://a.com', is_enabled=True)
        assert url.is_active() is True

        # Disabled
        url.is_enabled = False
        assert url.is_active() is False
        url.is_enabled = True

        # start_at in future
        url.start_at = now + timedelta(hours=1)
        assert url.is_active() is False

        # start_at in past
        url.start_at = now - timedelta(hours=1)
        assert url.is_active() is True

        # end_at in past
        url.end_at = now - timedelta(minutes=1)
        assert url.is_active() is False

        # end_at in future
        url.end_at = now + timedelta(hours=1)
        assert url.is_active() is True

        # expires_at in past
        url.expires_at = now - timedelta(seconds=1)
        assert url.is_active() is False

        # expires_at in future
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

def test_url_rotate_targets_edge_cases(app):
    with app.app_context():
        url = URL(short_code='edge', long_url='https://main.com')

        # Test getter with invalid JSON in DB
        url._rotate_targets = "not a json"
        assert url.rotate_targets == []

        # Test setter with non-list JSON string
        url.rotate_targets = json.dumps("not a list")
        assert url.rotate_targets == [json.dumps("not a list")]

        # Test setter with other iterables (e.g. generator)
        def gen():
            yield "https://gen1.com"
            yield "https://gen2.com"
        url.rotate_targets = gen()
        assert url.rotate_targets == ["https://gen1.com", "https://gen2.com"]

        # Test setter with non-iterable
        url.rotate_targets = 123
        assert url.rotate_targets == []
        assert url._rotate_targets is None
