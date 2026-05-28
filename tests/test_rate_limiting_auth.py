
def test_login_rate_limiting(app, client):
    # Enable rate limiting for testing
    app.config['RATELIMIT_ENABLED'] = True
    # Set a very low rate limit for testing
    app.config['RATELIMIT_LOGIN'] = '1 per minute'

    # First request should succeed (200 OK)
    response = client.get('/login')
    assert response.status_code == 200

    # Second request should be rate limited (429 Too Many Requests)
    response = client.get('/login')
    assert response.status_code == 429

def test_register_rate_limiting(app, client):
    # Enable rate limiting for testing
    app.config['RATELIMIT_ENABLED'] = True
    # Set a very low rate limit for testing
    app.config['RATELIMIT_REGISTER'] = '1 per minute'

    # First request should succeed (200 OK)
    response = client.get('/register')
    assert response.status_code == 200

    # Second request should be rate limited (429 Too Many Requests)
    response = client.get('/register')
    assert response.status_code == 429
