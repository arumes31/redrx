import io
import csv
from werkzeug.security import generate_password_hash
from app.models import db, URL, User

def test_export_links_csv_injection(app, client):
    # 1. Create a user
    with app.app_context():
        user = User(
            username='csvuser',
            email='csv@example.com',
            password_hash=generate_password_hash('password')
        )
        db.session.add(user)
        db.session.commit()

        # 2. Create URLs with malicious payloads
        u1 = URL(
            user_id=user.id,
            short_code='INJECT1',
            long_url='=SUM(1+1)',
            clicks_count=5
        )
        u2 = URL(
            user_id=user.id,
            short_code='INJECT2',
            long_url='+1+2',
            clicks_count=10
        )
        u3 = URL(
            user_id=user.id,
            short_code='INJECT3',
            long_url='-5-6',
            clicks_count=15
        )
        u4 = URL(
            user_id=user.id,
            short_code='INJECT4',
            long_url='@something',
            clicks_count=20
        )
        db.session.add_all([u1, u2, u3, u4])
        db.session.commit()

    # Re-login via client to ensure session is active for the client
    client.post('/login', data={'username': 'csvuser', 'password': 'password'}, follow_redirects=True)

    # 3. Request the export-links endpoint
    response = client.get('/export-links')
    assert response.status_code == 200
    assert response.mimetype == 'text/csv'

    # 4. Parse the CSV and check for escaping (vulnerability fix confirmation)
    csv_data = response.data.decode('utf-8')
    reader = csv.reader(io.StringIO(csv_data))
    rows = list(reader)

    # Header: Short Code, Long URL, Clicks, Created At, Last Accessed, Expires At
    # Rows start from index 1

    payloads = [row[1] for row in rows[1:]]
    assert "'=SUM(1+1)" in payloads
    assert "'+1+2" in payloads
    assert "'-5-6" in payloads
    assert "'@something" in payloads

    # Verify they ALL start with a single quote
    for p in payloads:
        assert p.startswith("'")
