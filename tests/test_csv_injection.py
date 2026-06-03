import io
import csv
import pytest
from app.models import db, URL

def test_csv_export_sanitization(client, app, test_user):
    with app.app_context():
        # Test cases for injection
        injection_urls = [
            "=SUM(1,2)",
            "+1+1",
            "-1-1",
            "@something"
        ]

        for i, long_url in enumerate(injection_urls):
            u = URL(
                long_url=long_url,
                short_code=f"INJ{i}",
                user_id=test_user.id
            )
            db.session.add(u)
        db.session.commit()

        # Log in the user
        with client.session_transaction() as sess:
            sess['_user_id'] = str(test_user.id)
            sess['_fresh'] = True

        # Request the export
        response = client.get('/export-links')
        assert response.status_code == 200
        assert response.mimetype == 'text/csv'

        # Parse CSV
        csv_data = response.data.decode('utf-8')
        reader = csv.reader(io.StringIO(csv_data))
        rows = list(reader)

        # Check headers
        assert rows[0] == ['Short Code', 'Long URL', 'Clicks', 'Created At', 'Last Accessed', 'Expires At']

        # Check for escaping
        found_count = 0
        for row in rows[1:]:
            long_url = row[1]
            if long_url.startswith("'"):
                original = long_url[1:]
                if original in injection_urls:
                    found_count += 1

        assert found_count == len(injection_urls)
