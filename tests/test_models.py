import json
from datetime import datetime, timezone, timedelta
import pytest
from app.models import db, User, URL, Click

def test_user_model_creation(app):
    """Test User model creation and attributes."""
    with app.app_context():
        user = User(
            username='newuser',
            email='new@example.com',
            password_hash='hash123',
            api_key='new-api-key'
        )
        db.session.add(user)
        db.session.commit()

        assert user.id is not None
        assert user.username == 'newuser'
        assert user.email == 'new@example.com'
        assert user.api_key == 'new-api-key'
        assert isinstance(user.created_at, datetime)

def test_user_urls_relationship(app):
    """Test the one-to-many relationship between User and URL."""
    with app.app_context():
        user = User(
            username='reluser',
            email='rel@example.com',
            password_hash='hash'
        )
        db.session.add(user)
        db.session.commit()

        url1 = URL(short_code='CODE1', long_url='https://url1.com', owner=user)
        url2 = URL(short_code='CODE2', long_url='https://url2.com', owner=user)
        db.session.add_all([url1, url2])
        db.session.commit()

        assert len(user.urls) == 2
        assert url1 in user.urls
        assert url2 in user.urls
        assert url1.user_id == user.id

def test_url_model_rotate_targets(app):
    """Test URL model rotate_targets property and setter."""
    with app.app_context():
        # Test with a list
        targets = ['https://target1.com', 'https://target2.com']
        url = URL(short_code='ROTATE', long_url='https://base.com', rotate_targets=targets)
        db.session.add(url)
        db.session.commit()

        # Reload from DB
        db.session.refresh(url)
        assert url.rotate_targets == targets
        assert isinstance(url._rotate_targets, str)
        assert json.loads(url._rotate_targets) == targets

        # Test setter with None/empty
        url.rotate_targets = []
        db.session.commit()
        assert url.rotate_targets == []
        assert url._rotate_targets is None

        url.rotate_targets = None
        db.session.commit()
        assert url.rotate_targets == []
        assert url._rotate_targets is None

def test_url_model_is_active(app):
    """Test URL model is_active logic."""
    with app.app_context():
        now = datetime.now(timezone.utc).replace(tzinfo=None)

        # Basic active URL (transient object needs explicit values for defaults)
        url = URL(short_code='ACTIVE', long_url='https://google.com', is_enabled=True)
        assert url.is_active() is True

        # Disabled URL
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
        url.end_at = now - timedelta(hours=1)
        assert url.is_active() is False
        # end_at in future
        url.end_at = now + timedelta(hours=1)
        assert url.is_active() is True

        # expires_at in past
        url.expires_at = now - timedelta(hours=1)
        assert url.is_active() is False
        # expires_at in future
        url.expires_at = now + timedelta(hours=1)
        assert url.is_active() is True

def test_click_model(app):
    """Test Click model creation and relationship to URL."""
    with app.app_context():
        url = URL(short_code='CLICKY', long_url='https://google.com')
        db.session.add(url)
        db.session.commit()

        click = Click(
            url_id=url.id,
            ip_address='127.0.0.1',
            country='Localhost',
            browser='Firefox',
            platform='Linux',
            referrer='Direct'
        )
        db.session.add(click)
        db.session.commit()

        assert click.id is not None
        assert click.url_id == url.id
        assert click.url == url
        assert len(url.clicks) == 1
        assert url.clicks[0] == click
