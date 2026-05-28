import pytest
from app.models import db, URL, Click
from datetime import datetime, timedelta, timezone

def test_stats_access_owner(client, app, test_user):
    with app.app_context():
        # Create a URL owned by test_user
        url = URL(short_code='OWNED', long_url='https://example.com', user_id=test_user.id)
        db.session.add(url)
        db.session.commit()

        # Create some clicks
        c1 = Click(url_id=url.id, country='US', browser='Chrome', platform='Windows', timestamp=datetime.now(timezone.utc) - timedelta(hours=1))
        db.session.add(c1)
        db.session.commit()

    # Need to be logged in
    with client.session_transaction() as sess:
        sess['_user_id'] = str(test_user.id)
        sess['_fresh'] = True

    response = client.get('/OWNED/stats')
    assert response.status_code == 200
    assert b'OWNED' in response.data
    assert b'Chrome' in response.data

def test_stats_access_anonymous_url(client, app):
    with app.app_context():
        # Create an anonymous URL
        url = URL(short_code='ANON', long_url='https://example.com', user_id=None)
        db.session.add(url)
        db.session.commit()

    response = client.get('/ANON/stats')
    assert response.status_code == 200

def test_stats_access_forbidden(client, app, test_user):
    with app.app_context():
        # Create a URL owned by another user (id 999)
        url = URL(short_code='OTHER', long_url='https://example.com', user_id=999)
        db.session.add(url)
        db.session.commit()

    # Log in as test_user (id matches test_user.id from fixture)
    with client.session_transaction() as sess:
        sess['_user_id'] = str(test_user.id)
        sess['_fresh'] = True

    response = client.get('/OTHER/stats')
    assert response.status_code == 403

def test_stats_time_ranges(client, app, test_user):
    with app.app_context():
        url = URL(short_code='RANGE', long_url='https://example.com', user_id=None)
        db.session.add(url)
        db.session.commit()

        # Click 2 hours ago
        c1 = Click(url_id=url.id, timestamp=datetime.now(timezone.utc) - timedelta(hours=2), browser='Chrome', platform='Windows')
        # Click 2 days ago
        c2 = Click(url_id=url.id, timestamp=datetime.now(timezone.utc) - timedelta(days=2), browser='Firefox', platform='Linux')
        db.session.add_all([c1, c2])
        db.session.commit()

    # Test 24h range
    response = client.get('/RANGE/stats?range=24h')
    assert response.status_code == 200
    # Average daily should be 1.0 (1 click in 24h range)
    assert b'1.0' in response.data

    # Test 7d range
    response = client.get('/RANGE/stats?range=7d')
    assert response.status_code == 200
    # 2 clicks in 7d range. avg = 2/7 = 0.285... rounded to 0.3
    assert b'0.3' in response.data
