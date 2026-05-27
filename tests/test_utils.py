import pytest
from app.utils import select_rotate_target

def test_select_rotate_target_empty():
    assert select_rotate_target([]) is None
    assert select_rotate_target(None) is None
    assert select_rotate_target("") is None

def test_select_rotate_target_list():
    targets = ["http://a.com", "http://b.com"]
    result = select_rotate_target(targets)
    assert result in targets

def test_select_rotate_target_string():
    # Currently this fails (returns a character)
    target = "http://example.com"
    assert select_rotate_target(target) == target

def test_select_rotate_target_set():
    # Currently this fails (TypeError: 'set' object is not subscriptable)
    targets = {"http://a.com", "http://b.com"}
    result = select_rotate_target(targets)
    assert result in targets
