import pytest
from app.models import db, URL

def test_stats_access_owner(client, app, test_user):
    # Owner should have access
    with client.session_transaction() as sess:
        sess['_user_id'] = test_user.id
        sess['_fresh'] = True

    with app.app_context():
        url = URL(short_code='owned', long_url='https://example.com', user_id=test_user.id)
        db.session.add(url)
        db.session.commit()

    response = client.get('/owned/stats')
    assert response.status_code == 200

def test_stats_access_anonymous_visitor(client, app, test_user):
    # Unauthenticated visitor should be blocked from owned URL stats
    with app.app_context():
        url = URL(short_code='owned2', long_url='https://example.com', user_id=test_user.id)
        db.session.add(url)
        db.session.commit()

    response = client.get('/owned2/stats')
    assert response.status_code == 403

def test_stats_access_different_user(client, app, test_user):
    # Different authenticated user should be blocked from owned URL stats
    with app.app_context():
        url = URL(short_code='owned3', long_url='https://example.com', user_id=test_user.id)
        db.session.add(url)
        db.session.commit()

    # Log in as a different user ID (e.g. 9999)
    with client.session_transaction() as sess:
        sess['_user_id'] = 9999
        sess['_fresh'] = True

    response = client.get('/owned3/stats')
    assert response.status_code == 403

def test_stats_access_anonymous_link(client, app, test_user):
    # Stats of anonymous-created link should be forbidden to everyone
    with app.app_context():
        # URL created anonymously (user_id is None)
        url = URL(short_code='anon', long_url='https://example.com', user_id=None)
        db.session.add(url)
        db.session.commit()

    # 1. Accessing anonymously
    response = client.get('/anon/stats')
    assert response.status_code == 403

    # 2. Accessing while logged in
    with client.session_transaction() as sess:
        sess['_user_id'] = test_user.id
        sess['_fresh'] = True

    response = client.get('/anon/stats')
    assert response.status_code == 403
