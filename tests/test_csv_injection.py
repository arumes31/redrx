import pytest
from app.utils import sanitize_csv_field

def test_sanitize_csv_field():
    assert sanitize_csv_field("=SUM(1,2)") == "'=SUM(1,2)"
    assert sanitize_csv_field("+123") == "'+123"
    assert sanitize_csv_field("-456") == "'-456"
    assert sanitize_csv_field("@something") == "'@something"
    assert sanitize_csv_field("normal") == "normal"
    assert sanitize_csv_field(123) == 123
    assert sanitize_csv_field(None) is None
    assert sanitize_csv_field("") == ""
