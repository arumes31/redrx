import pytest
from app.models import db, URL, User
from werkzeug.security import generate_password_hash

def test_csv_export_sanitization(app, client):
    # 1. Create a user and log in
    with app.app_context():
        user = User(
            username='csvuser',
            email='csv@example.com',
            password_hash=generate_password_hash('password')
        )
        db.session.add(user)
        db.session.commit()

        # 2. Create a URL with a malicious long_url (CSV injection)
        malicious_url = URL(
            short_code='DANGER',
            long_url='=SUM(1,2)',
            user_id=user.id
        )
        db.session.add(malicious_url)
        db.session.commit()

        # Log in the user
        with client.session_transaction() as sess:
            sess['_user_id'] = str(user.id)
            sess['_fresh'] = True

    # 3. Export links
    response = client.get('/export-links')
    assert response.status_code == 200

    csv_data = response.data.decode('utf-8')

    # Check if it's sanitized.
    # The value should contain the prepended single quote.
    assert "'=SUM(1,2)" in csv_data

    # Verify it was applied to short_code too if it was malicious
    with app.app_context():
        malicious_code = URL(
            short_code='+ATTACK',
            long_url='https://example.com',
            user_id=user.id
        )
        db.session.add(malicious_code)
        db.session.commit()

    response = client.get('/export-links')
    csv_data = response.data.decode('utf-8')
    assert "'+ATTACK" in csv_data
