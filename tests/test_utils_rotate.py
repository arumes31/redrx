from app.utils import select_rotate_target

def test_select_rotate_target_empty():
    assert select_rotate_target([]) is None
    assert select_rotate_target(None) is None
    assert select_rotate_target("") is None

def test_select_rotate_target_single_string():
    url = "https://example.com"
    assert select_rotate_target(url) == url

def test_select_rotate_target_list():
    targets = ["https://a.com", "https://b.com"]
    result = select_rotate_target(targets)
    assert result in targets

def test_select_rotate_target_other_iterable():
    targets = {"https://a.com", "https://b.com"}
    result = select_rotate_target(targets)
    assert result in targets

def test_select_rotate_target_empty_generator():
    def empty_gen():
        yield from []
    assert select_rotate_target(empty_gen()) is None
