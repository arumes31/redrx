# Sentinel Security Journal

## Critical Security Learnings

- **Open Redirect in Login**: Discovered that the `next` parameter in the login route was being used directly in a `redirect()` call without validation. Fixed by implementing `is_safe_redirect_url` which ensures the target is a relative path starting with a single `/`.
- **SQLAlchemy DetachedInstanceError**: Encountered `DetachedInstanceError` in tests when accessing attributes of an expunged object. Resolved by accessing necessary attributes (like `id`) before expunging them from the session in the fixture.
