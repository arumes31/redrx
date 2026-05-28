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

def test_url_rotate_targets(app):
    with app.app_context():
        # Test setting list
        targets = ['https://site1.com', 'https://site2.com']
        url = URL(short_code='rotate', long_url='https://main.com')
        url.rotate_targets = targets
        db.session.add(url)
        db.session.commit()

        saved_url = URL.query.filter_by(short_code='rotate').first()
        assert saved_url.rotate_targets == targets
        import json
        assert json.loads(saved_url._rotate_targets) == targets

        # Test setting None
        url.rotate_targets = None
        db.session.commit()
        assert saved_url.rotate_targets == []
        assert saved_url._rotate_targets is None

        # Test setting empty list
        url.rotate_targets = []
        db.session.commit()
        assert saved_url.rotate_targets == []
        assert saved_url._rotate_targets is None

def test_url_is_active(app):
    with app.app_context():
        now = datetime.now(timezone.utc).replace(tzinfo=None)

        # Manually set is_enabled=True as it's a default in DB but not in __init__
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
