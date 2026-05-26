import pytest
import io
import csv
from app.models import db, URL, User

def test_export_links_csv_injection_mitigated(app, client):
    # 1. Setup: Create a user and a malicious URL
    with app.app_context():
        user = User(
            username='csvuser',
            email='csv@example.com',
            password_hash='pbkdf2:sha256:260000$8mX...dummy'
        )
        db.session.add(user)
        db.session.commit()

        malicious_url = URL(
            short_code='=DANGER',
            long_url='=SUM(1+2)',
            user_id=user.id
        )
        db.session.add(malicious_url)
        db.session.commit()

        user_id = user.id

    # 2. Login the user
    with client.session_transaction() as sess:
        sess['_user_id'] = str(user_id)
        sess['_fresh'] = True

    # 3. Call export-links
    response = client.get('/export-links')
    assert response.status_code == 200
    assert response.mimetype == 'text/csv'

    # 4. Verify mitigation
    data = response.data.decode('utf-8')
    reader = csv.reader(io.StringIO(data))
    rows = list(reader)

    found_sanitized_long = False
    found_sanitized_short = False
    for row in rows:
        if "'=SUM(1+2)" in row:
            found_sanitized_long = True
        if "'=DANGER" in row:
            found_sanitized_short = True

    assert found_sanitized_long, f"Long URL payload not sanitized: {rows}"
    assert found_sanitized_short, f"Short code payload not sanitized: {rows}"
