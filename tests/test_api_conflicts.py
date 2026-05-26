import pytest
from unittest.mock import patch
from app.models import db, URL


def test_custom_code_conflict(app, client, test_user):
    """Test that a 409 error is returned when a custom code is already taken."""
    # First, create a URL with a custom code
    with app.app_context():
        url = URL(short_code='TAKEN', long_url='https://example.com', user_id=test_user.id)
        db.session.add(url)
        db.session.commit()

    # Now attempt to shorten another URL using the same custom code
    response = client.post('/api/v1/shorten',
                           headers={'X-API-KEY': 'test-api-key'},
                           json={
                               'long_url': 'https://another.com',
                               'custom_code': 'taken'  # Case-insensitive check in app/api.py
                           })

    assert response.status_code == 409
    assert response.get_json()['error'] == 'Custom code already taken'

def test_autogen_code_collision(app, client, test_user):
    """Test that the app retries generation when an auto-generated code collides."""
    # First, create a URL with a known short code
    with app.app_context():
        url = URL(short_code='COLLIDE', long_url='https://example.com', user_id=test_user.id)
        db.session.add(url)
        db.session.commit()

    # Mock generate_short_code to return 'COLLIDE' first, then 'SUCCESS'
    with patch('app.api.generate_short_code') as mock_gen:
        mock_gen.side_effect = ['COLLIDE', 'SUCCESS']

        response = client.post('/api/v1/shorten',
                               headers={'X-API-KEY': 'test-api-key'},
                               json={'long_url': 'https://target.com'})

        assert response.status_code == 201
        data = response.get_json()
        assert data['short_code'] == 'SUCCESS'
        # Ensure it was called twice
        assert mock_gen.call_count == 2
