import pytest
from app.models import db, URL

def test_bulk_delete_success(app, auth_client, test_user):
    # Create some links for the test user
    with app.app_context():
        u1 = URL(short_code='link1', long_url='https://example.com/1', user_id=test_user.id)
        u2 = URL(short_code='link2', long_url='https://example.com/2', user_id=test_user.id)
        u3 = URL(short_code='link3', long_url='https://example.com/3', user_id=test_user.id)
        db.session.add_all([u1, u2, u3])
        db.session.commit()

        # Capture IDs
        ids = [str(u1.id), str(u2.id)]

    # Perform bulk delete
    response = auth_client.post('/bulk-delete', data={'link_ids': ids}, follow_redirects=True)

    assert response.status_code == 200
    assert b'Successfully deleted 2 links.' in response.data

    # Verify they are gone
    with app.app_context():
        remaining = URL.query.filter(URL.user_id == test_user.id).all()
        assert len(remaining) == 1
        assert remaining[0].short_code == 'link3'

def test_bulk_delete_other_user_links(app, auth_client, test_user, other_user):
    # Create links for both users
    with app.app_context():
        u1 = URL(short_code='my-link', long_url='https://example.com/mine', user_id=test_user.id)
        u2 = URL(short_code='other-link', long_url='https://example.com/other', user_id=other_user.id)
        db.session.add_all([u1, u2])
        db.session.commit()

        my_id = u1.id
        other_id = u2.id

    # Try to delete both
    response = auth_client.post('/bulk-delete', data={'link_ids': [str(my_id), str(other_id)]}, follow_redirects=True)

    assert response.status_code == 200
    assert b'Successfully deleted 2 links.' in response.data

    # Verify only own link was deleted
    with app.app_context():
        # My link should be gone
        assert db.session.get(URL, my_id) is None
        # Other user's link should still be there
        assert db.session.get(URL, other_id) is not None

def test_bulk_delete_no_selection(auth_client):
    response = auth_client.post('/bulk-delete', data={}, follow_redirects=True)

    assert response.status_code == 200
    assert b'No links selected.' in response.data

def test_bulk_delete_unauthenticated(client):
    response = client.post('/bulk-delete', data={'link_ids': ['1', '2']}, follow_redirects=True)

    # Flask-Login redirects to login page
    assert b'Please log in to access this page.' in response.data
