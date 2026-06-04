from unittest.mock import patch
from app.models import db, URL


def test_api_shorten_custom_code_collision(client, test_user):
    # Setup: Create a URL with a known short code
    headers = {"X-API-KEY": "test-api-key"}
    setup_response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={"long_url": "https://example.com/1", "custom_code": "TAKEN"},
    )
    assert setup_response.status_code == 201
    assert setup_response.get_json()["short_code"] == "TAKEN"

    # Action: Try to use the same custom code
    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={"long_url": "https://example.com/2", "custom_code": "TAKEN"},
    )

    # Verification
    assert response.status_code == 409
    assert response.get_json()["error"] == "Custom code already taken"


def test_api_shorten_generated_code_collision(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}

    # Pre-populate the DB with a code that will collide
    with client.application.app_context():
        url = URL(
            short_code="COLLIDE", long_url="https://initial.com", user_id=test_user.id
        )
        db.session.add(url)
        db.session.commit()

    # Mock generate_short_code to return 'COLLIDE' then 'UNIQUE'
    with patch("app.api.generate_short_code") as mock_gen:
        mock_gen.side_effect = ["COLLIDE", "UNIQUE"]

        response = client.post(
            "/api/v1/shorten",
            headers=headers,
            json={"long_url": "https://example.com/unique"},
        )

        assert response.status_code == 201
        data = response.get_json()
        assert data["short_code"] == "UNIQUE"
        assert mock_gen.call_count == 2


def test_api_shorten_valid_dates(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    start_at = "2025-01-01T00:00:00Z"
    end_at = "2025-12-31T23:59:59+00:00"

    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={
            "long_url": "https://example.com",
            "start_at": start_at,
            "end_at": end_at,
        },
    )

    assert response.status_code == 201
    data = response.get_json()
    assert data["start_at"] == "2025-01-01T00:00:00+00:00"
    assert data["end_at"] == "2025-12-31T23:59:59+00:00"


def test_api_shorten_invalid_start_at(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={"long_url": "https://example.com", "start_at": "invalid-date"},
    )

    assert response.status_code == 400
    assert response.get_json()["error"] == "Invalid start_at format. Use ISO 8601"


def test_api_shorten_invalid_end_at(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={"long_url": "https://example.com", "end_at": "invalid-date"},
    )

    assert response.status_code == 400
    assert response.get_json()["error"] == "Invalid end_at format. Use ISO 8601"


def test_api_shorten_invalid_window(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={
            "long_url": "https://example.com",
            "start_at": "2025-12-31T23:59:59Z",
            "end_at": "2025-01-01T00:00:00Z",
        },
    )

    assert response.status_code == 400
    assert (
        response.get_json()["error"]
        == "Invalid scheduling window: end_at must be after start_at"
    )


def test_api_shorten_validate_password(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={"long_url": "https://example.com", "password": 12345},
    )
    assert response.status_code == 400
    assert response.get_json()["error"] == "password must be a string"


def test_api_shorten_validate_preview_mode(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={"long_url": "https://example.com", "preview_mode": "invalid_bool"},
    )
    assert response.status_code == 400
    assert response.get_json()["error"] == "preview_mode must be a boolean"

    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={"long_url": "https://example.com", "preview_mode": "false"},
    )
    assert response.status_code == 201
    assert response.get_json()["preview_mode"] is False


def test_api_shorten_validate_stats_enabled(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={"long_url": "https://example.com", "stats_enabled": "invalid_bool"},
    )
    assert response.status_code == 400
    assert response.get_json()["error"] == "stats_enabled must be a boolean"

    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={"long_url": "https://example.com", "stats_enabled": "1"},
    )
    assert response.status_code == 201
    assert response.get_json()["stats_enabled"] is True


def test_api_shorten_custom_code_too_short(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={"long_url": "https://example.com", "custom_code": "AB"},
    )
    assert response.status_code == 400
    assert (
        response.get_json()["error"]
        == "custom_code must be between 3 and 20 characters"
    )


def test_api_shorten_custom_code_too_long(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={"long_url": "https://example.com", "custom_code": "A" * 21},
    )
    assert response.status_code == 400
    assert (
        response.get_json()["error"]
        == "custom_code must be between 3 and 20 characters"
    )


def test_api_shorten_custom_code_invalid_chars(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={"long_url": "https://example.com", "custom_code": "INVALID CODE!"},
    )
    assert response.status_code == 400
    assert (
        response.get_json()["error"]
        == "custom_code must contain only alphanumeric characters, hyphens, or underscores"
    )


def test_api_shorten_custom_code_not_string(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={"long_url": "https://example.com", "custom_code": 123},
    )
    assert response.status_code == 400
    assert response.get_json()["error"] == "custom_code must be a string"


def test_api_shorten_code_length_invalid_range(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={"long_url": "https://example.com", "code_length": 2},
    )
    assert response.status_code == 400
    assert response.get_json()["error"] == "code_length must be between 3 and 20"

    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={"long_url": "https://example.com", "code_length": 21},
    )
    assert response.status_code == 400
    assert response.get_json()["error"] == "code_length must be between 3 and 20"


def test_api_shorten_code_length_not_integer(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={"long_url": "https://example.com", "code_length": "not-an-int"},
    )
    assert response.status_code == 400
    assert response.get_json()["error"] == "code_length must be an integer"


def test_api_shorten_multiple_collisions(client, test_user, app):
    headers = {"X-API-KEY": "test-api-key"}

    with app.app_context():
        u1 = URL(short_code="COLL1", long_url="https://ex1.com")
        u2 = URL(short_code="COLL2", long_url="https://ex2.com")
        db.session.add_all([u1, u2])
        db.session.commit()

    with patch("app.api.generate_short_code") as mock_gen:
        # It should try COLL1, then COLL2, then finally succeed with UNIQUE
        mock_gen.side_effect = ["COLL1", "COLL2", "UNIQUE"]

        response = client.post(
            "/api/v1/shorten",
            headers=headers,
            json={"long_url": "https://example.com", "code_length": 5},
        )

        assert response.status_code == 201
        assert response.get_json()["short_code"] == "UNIQUE"
        assert mock_gen.call_count == 3
        # Verify code_length was passed each time
        mock_gen.assert_called_with(5)


def test_api_shorten_custom_code_normalization(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={"long_url": "https://example.com", "custom_code": "  my-code  "},
    )
    assert response.status_code == 201
    assert response.get_json()["short_code"] == "MY-CODE"


def test_api_shorten_invalid_rotate_targets(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={"long_url": "https://example.com", "rotate_targets": "not-a-list"},
    )
    assert response.status_code == 400
    assert response.get_json()["error"] == "rotate_targets must be a list of strings"

    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={"long_url": "https://example.com", "rotate_targets": [123]},
    )
    assert response.status_code == 400
    assert response.get_json()["error"] == "rotate_targets must be a list of strings"


def test_api_shorten_too_many_rotate_targets(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={
            "long_url": "https://example.com",
            "rotate_targets": ["https://ex.com"] * 51,
        },
    )
    assert response.status_code == 400
    assert response.get_json()["error"] == "Maximum 50 rotate targets allowed"


def test_api_shorten_blocked_rotate_targets(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    with patch("app.api.is_safe_url") as mock_safe:
        mock_safe.side_effect = [True, False]  # long_url safe, rotate_target unsafe
        response = client.post(
            "/api/v1/shorten",
            headers=headers,
            json={
                "long_url": "https://safe.com",
                "rotate_targets": ["https://unsafe.com"],
            },
        )
        assert response.status_code == 403
        assert (
            response.get_json()["error"]
            == "One or more rotate target URLs are blocked or invalid."
        )


def test_api_shorten_payload_not_dict(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post("/api/v1/shorten", headers=headers, json=["not-a-dict"])
    assert response.status_code == 400
    assert response.get_json()["error"] == "Request payload must be a JSON object"


def test_api_shorten_missing_long_url(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post("/api/v1/shorten", headers=headers, json={})
    assert response.status_code == 400
    assert response.get_json()["error"] == "Missing long_url"


def test_api_shorten_long_url_not_string(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post("/api/v1/shorten", headers=headers, json={"long_url": 123})
    assert response.status_code == 400
    assert response.get_json()["error"] == "long_url must be a string"


def test_api_shorten_blocked_long_url(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    with patch("app.api.is_safe_url", return_value=False):
        response = client.post(
            "/api/v1/shorten", headers=headers, json={"long_url": "https://blocked.com"}
        )
        assert response.status_code == 403
        assert response.get_json()["error"] == "Destination URL is blocked"


def test_api_shorten_expiry_out_of_range(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={"long_url": "https://example.com", "expiry_hours": 1000000},
    )
    assert response.status_code == 400
    assert "expiry_hours must be between 0 and 876,000" in response.get_json()["error"]


def test_api_shorten_expiry_not_integer(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={"long_url": "https://example.com", "expiry_hours": "not-an-int"},
    )
    assert response.status_code == 400
    assert response.get_json()["error"] == "expiry_hours must be an integer"


def test_api_shorten_expiry_zero(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={"long_url": "https://example.com", "expiry_hours": 0},
    )
    assert response.status_code == 201
    assert response.get_json()["expires_at"] is None


def test_api_shorten_invalid_scheduling_type(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={"long_url": "https://example.com", "start_at": 123},
    )
    assert response.status_code == 400
    assert response.get_json()["error"] == "scheduling dates must be strings (ISO 8601)"


def test_api_shorten_empty_password(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={"long_url": "https://example.com", "password": ""},
    )
    assert response.status_code == 201
    assert response.get_json()["password_protected"] is False


def test_api_shorten_with_rotate_targets_success(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={
            "long_url": "https://example.com",
            "rotate_targets": ["https://a.com", "https://b.com"],
        },
    )
    assert response.status_code == 201
    assert response.get_json()["rotate_targets"] == ["https://a.com", "https://b.com"]


def test_api_shorten_boolean_strings(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={
            "long_url": "https://example.com",
            "preview_mode": "yes",
            "stats_enabled": "no",
        },
    )
    assert response.status_code == 201
    assert response.get_json()["preview_mode"] is True
    assert response.get_json()["stats_enabled"] is False


def test_api_shorten_boolean_strings_numeric(client, test_user):
    headers = {"X-API-KEY": "test-api-key"}
    response = client.post(
        "/api/v1/shorten",
        headers=headers,
        json={
            "long_url": "https://example.com",
            "preview_mode": "0",
            "stats_enabled": "1",
        },
    )
    assert response.status_code == 201
    assert response.get_json()["preview_mode"] is False
    assert response.get_json()["stats_enabled"] is True
