import pytest
from app.models import db, URL, Click, User
from datetime import datetime, timedelta, timezone

def test_stats_aggregation(client, app, test_user):
    user_id = test_user.id

    with app.app_context():
        db.session.add(test_user)

        url = URL(
            short_code='teststats',
            long_url='https://example.com',
            user_id=user_id
        )
        db.session.add(url)
        db.session.commit()

        # Add some clicks
        now = datetime.now(timezone.utc)
        clicks = [
            # Last 24h
            Click(url_id=url.id, country='US', browser='Chrome', platform='Windows', referrer='https://google.com', timestamp=now - timedelta(hours=1)),
            Click(url_id=url.id, country='US', browser='Firefox', platform='Windows', referrer='https://google.com', timestamp=now - timedelta(hours=2)),
            Click(url_id=url.id, country='UK', browser='Chrome', platform='MacOS', referrer='https://bing.com', timestamp=now - timedelta(hours=5)),

            # Older than 24h but within 7d
            Click(url_id=url.id, country='CA', browser='Safari', platform='iOS', referrer='https://t.co', timestamp=now - timedelta(days=2)),

            # Older than 7d but within 30d
            Click(url_id=url.id, country='FR', browser='Edge', platform='Linux', referrer=None, timestamp=now - timedelta(days=15)),
        ]
        db.session.add_all(clicks)
        db.session.commit()

        short_code = url.short_code

    # Mocking login for the test client
    with client.session_transaction() as sess:
        sess['_user_id'] = str(user_id)
        sess['_fresh'] = True

    # Test 24h range
    response = client.get(f'/{short_code}/stats?range=24h')
    assert response.status_code == 200
    html = response.data.decode()
    assert 'US' in html
    assert 'UK' in html
    assert 'Chrome' in html
    assert 'Firefox' in html
    assert 'Windows' in html
    assert 'MacOS' in html

    # Test 30d range
    response = client.get(f'/{short_code}/stats?range=30d')
    assert response.status_code == 200
    html = response.data.decode()
    assert 'CA' in html
    assert 'FR' in html
    assert 'Safari' in html
    assert 'Edge' in html
