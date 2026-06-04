import pytest
from app.models import db, URL, User
from flask import url_for

def test_bulk_delete_requires_login(client):
    response = client.post('/bulk-delete', data={'link_ids': ['1', '2']})
    assert response.status_code == 302
    assert '/login' in response.location

def test_bulk_delete_success(client, app, test_user):
    with app.app_context():
        # Create some URLs for the test user
        u1 = URL(short_code='url1', long_url='https://example1.com', user_id=test_user.id)
        u2 = URL(short_code='url2', long_url='https://example2.com', user_id=test_user.id)
        u3 = URL(short_code='url3', long_url='https://example3.com', user_id=test_user.id)
        db.session.add_all([u1, u2, u3])
        db.session.commit()
        id1, id2, id3 = u1.id, u2.id, u3.id

    # Log in
    with client.session_transaction() as sess:
        sess['_user_id'] = str(test_user.id)
        sess['_fresh'] = True

    # Delete two of them
    response = client.post('/bulk-delete', data={'link_ids': [str(id1), str(id2)]}, follow_redirects=True)
    assert response.status_code == 200
    assert b'Successfully deleted 2 links' in response.data

    with app.app_context():
        assert db.session.get(URL, id1) is None
        assert db.session.get(URL, id2) is None
        assert db.session.get(URL, id3) is not None

def test_bulk_delete_unauthorized_flash_count(client, app, test_user):
    with app.app_context():
        # Create another user
        other_user = User(username='other', email='other@example.com', password_hash='hash')
        db.session.add(other_user)
        db.session.commit()

        # Create a URL for the other user
        u_other = URL(short_code='other', long_url='https://other.com', user_id=other_user.id)
        # Create a URL for the test user
        u_test = URL(short_code='test', long_url='https://test.com', user_id=test_user.id)
        db.session.add_all([u_other, u_test])
        db.session.commit()
        id_other = u_other.id
        id_test = u_test.id

    # Log in as test_user
    with client.session_transaction() as sess:
        sess['_user_id'] = str(test_user.id)
        sess['_fresh'] = True

    # Try to delete both. Only 1 should actually be deleted.
    response = client.post('/bulk-delete', data={'link_ids': [str(id_other), str(id_test)]}, follow_redirects=True)
    assert response.status_code == 200
    # This will FAIL currently because it will say "Successfully deleted 2 links"
    assert b'Successfully deleted 1 links' in response.data

    with app.app_context():
        assert db.session.get(URL, id_other) is not None # Should NOT be deleted
        assert db.session.get(URL, id_test) is None # Should be deleted

def test_bulk_delete_no_selection(client, test_user):
    # Log in
    with client.session_transaction() as sess:
        sess['_user_id'] = str(test_user.id)
        sess['_fresh'] = True

    response = client.post('/bulk-delete', data={}, follow_redirects=True)
    assert response.status_code == 200
    assert b'No links selected.' in response.data
