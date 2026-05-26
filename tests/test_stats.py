import pytest
from app.models import db, URL, Click
from datetime import datetime, timedelta, timezone

def test_stats_anonymous(client, app):
    """Test that stats are accessible for anonymous links."""
    with app.app_context():
        url = URL(long_url='https://google.com', short_code='ANON1', user_id=None)
        db.session.add(url)
        db.session.commit()
        url_id = url.id

        # Add some clicks
        click1 = Click(
            url_id=url_id,
            ip_address='127.0.0.1',
            browser='Chrome',
            platform='Windows',
            country='US',
            timestamp=datetime.now(timezone.utc) - timedelta(hours=1)
        )
        # IPv6 click
        click2 = Click(
            url_id=url_id,
            ip_address='2001:0db8:85a3:0000:0000:8a2e:0370:7334',
            browser='Firefox',
            platform='Linux',
            country='GB',
            timestamp=datetime.now(timezone.utc) - timedelta(days=2)
        )
        db.session.add_all([click1, click2])
        db.session.commit()

    # Test stats page
    response = client.get('/ANON1/stats')
    assert response.status_code == 200
    assert b'ANON1' in response.data
    assert b'Chrome' in response.data
    assert b'Firefox' in response.data
    # Verify IP anonymization
    assert b'127.0.xxx.xxx' in response.data
    assert b'2001:0db8:xxxx:xxxx' in response.data

def test_stats_unauthorized(client, app):
    """Test that stats are restricted for owned links."""
    with app.app_context():
        # Create URL owned by user ID 1
        url = URL(long_url='https://google.com', short_code='PRIVATE', user_id=1)
        db.session.add(url)
        db.session.commit()

    # Not logged in, should be 403
    response = client.get('/PRIVATE/stats')
    assert response.status_code == 403

def test_stats_range_24h(client, app):
    """Test stats with 24h range filter."""
    with app.app_context():
        url = URL(long_url='https://google.com', short_code='RANGE24', user_id=None)
        db.session.add(url)
        db.session.commit()

    response = client.get('/RANGE24/stats?range=24h')
    assert response.status_code == 200
