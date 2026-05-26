import pytest
from app.models import db, URL, Click
from datetime import datetime, timedelta, timezone

def test_stats_data_consistency(client, app):
    # Create a test URL
    with app.app_context():
        url = URL(short_code='teststats', long_url='https://example.com')
        db.session.add(url)
        db.session.commit()
        url_id = url.id

        # Add some clicks
        now = datetime.now(timezone.utc)
        clicks = [
            Click(url_id=url_id, timestamp=now - timedelta(hours=1), country='US', browser='Chrome', platform='Windows', referrer='https://google.com/1'),
            Click(url_id=url_id, timestamp=now - timedelta(hours=2), country='US', browser='Firefox', platform='Windows', referrer='https://google.com/2'),
            Click(url_id=url_id, timestamp=now - timedelta(hours=25), country='UK', browser='Safari', platform='Mac', referrer='https://bing.com'),
            Click(url_id=url_id, timestamp=now - timedelta(days=2), country='DE', browser='Chrome', platform='Linux', referrer=None),
        ]
        db.session.add_all(clicks)
        db.session.commit()

    # Test 30d range (default)
    response = client.get('/teststats/stats?range=30d')
    assert response.status_code == 200
    html = response.data.decode()

    # Verify counts in charts
    assert 'US' in html
    assert 'UK' in html
    assert 'DE' in html
    assert 'google.com' in html
    assert 'bing.com' in html
    assert 'Direct' in html

    # Momentum: 4 clicks / 30 days = 0.1
    assert '0.1' in html

    # Test 24h range
    response = client.get('/teststats/stats?range=24h')
    assert response.status_code == 200
    html = response.data.decode()

    # Momentum: 2 clicks / 1 day = 2.0
    assert '2.0' in html
