import re
from urllib.parse import urlparse
import pytest
from app.models import db, URL, Click
from datetime import datetime, timedelta, timezone

def test_stats_data_consistency(client, app, test_user):
    # Log in the user
    with client.session_transaction() as sess:
        sess['_user_id'] = test_user.id
        sess['_fresh'] = True

    # Create a URL
    with app.app_context():
        url = URL(
            short_code='testperf',
            long_url='https://example.com',
            user_id=test_user.id
        )
        db.session.add(url)
        db.session.commit()
        url_id = url.id

        # Add some clicks
        now = datetime.now(timezone.utc)
        clicks = [
            Click(url_id=url_id, timestamp=now - timedelta(hours=1), country='US', browser='Chrome', platform='Windows', referrer='https://google.com/search'),
            Click(url_id=url_id, timestamp=now - timedelta(hours=2), country='US', browser='Firefox', platform='Linux', referrer='https://bing.com'),
            Click(url_id=url_id, timestamp=now - timedelta(days=2), country='UK', browser='Safari', platform='MacOS', referrer=None),
            Click(url_id=url_id, timestamp=now - timedelta(days=10), country='FR', browser='Chrome', platform='Android', referrer='Direct'),
        ]
        db.session.bulk_save_objects(clicks)
        db.session.commit()

    # Test stats for different ranges
    for range_type in ['24h', '7d', '30d']:
        response = client.get(f'/testperf/stats?range={range_type}')
        assert response.status_code == 200
        html = response.data.decode()
        assert "US" in html
        assert "Chrome" in html
        assert "Windows" in html
        urls_in_html = re.findall(r'https?://[^\s"\'<>]+', html)
        assert any(urlparse(u).hostname == "google.com" for u in urls_in_html)
        
        if range_type == '24h':
            assert "2.0" in html
        elif range_type == '7d':
            assert "0.4" in html
        elif range_type == '30d':
            assert "0.1" in html
