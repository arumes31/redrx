import pytest
from app.models import db, URL, User

def test_anonymous_stats_access(client, app):
    with app.app_context():
        # Create an anonymous URL
        url = URL(short_code='anon', long_url='https://example.com', user_id=None)
        db.session.add(url)
        db.session.commit()

    # Try to access stats as anonymous user
    response = client.get('/anon/stats')
    # If the logic in the description is followed, this should be 200.
    # But currently it might be 200 because None == None.
    assert response.status_code == 200

def test_authenticated_user_accesses_anonymous_stats(client, app, test_user):
    with app.app_context():
        # Create an anonymous URL
        url = URL(short_code='anon2', long_url='https://example.com', user_id=None)
        db.session.add(url)
        db.session.commit()

    # Log in as test_user
    with client.session_transaction() as sess:
        sess['_user_id'] = test_user.id
        sess['_fresh'] = True

    # Try to access stats of anonymous URL
    response = client.get('/anon2/stats')
    # CURRENTLY this will return 403 because url.user_id (None) != test_user.id (1)
    # But if "anyone can see stats" for anonymous links, it should be 200.
    assert response.status_code == 200

def test_unauthorized_access_to_private_stats(client, app, test_user):
    # Create another user
    with app.app_context():
        other_user = User(username='other', email='other@example.com', password_hash='hash')
        db.session.add(other_user)
        db.session.commit()
        other_user_id = other_user.id

        # Create a private URL for other user
        url = URL(short_code='private', long_url='https://example.com', user_id=other_user_id)
        db.session.add(url)
        db.session.commit()

    # Log in as test_user
    with client.session_transaction() as sess:
        sess['_user_id'] = test_user.id
        sess['_fresh'] = True

    # Try to access stats of private URL
    response = client.get('/private/stats')
    assert response.status_code == 403
