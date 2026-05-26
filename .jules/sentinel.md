## 2026-03-23 - Prevent Malicious URL Schemes in Link Shortener
**Vulnerability:** The link shortening API allowed shortening of non-HTTP(S) URLs like `javascript:` and `data:` schemes, leading to Stored XSS.
**Learning:** Even if frontend forms (like WTForms) use URL validators, the API layer must independently strictly validate the URL scheme (allowlisting `http` and `https`) to prevent malicious payloads from bypassing the UI.
**Prevention:** Always enforce a strict scheme allowlist (`['http', 'https']`) on user-provided URLs at the backend/API level before accepting and storing them as redirects.

## 2026-05-21 - Memory Efficiency in Large Database Iterations
**Issue:** Iterating over all records using `.all()` in a long-running maintenance task (phishing URL cleanup) can lead to Out-Of-Memory (OOM) errors as the database grows.
**Learning:** Use SQLAlchemy's `.yield_per()` to stream results from the database in chunks. To truly keep memory low, objects must be explicitly expunged from the session (`db.session.expunge(obj)`) after processing if they are not being modified/deleted, as SQLAlchemy's Identity Map otherwise keeps them all in memory.
**Prevention:** Always use chunked iteration (`yield_per`) and active session management (expunging or session clearing) for operations involving large result sets.

## 2026-05-21 - Fix DetachedInstanceError in Tests
**Issue:** Test fixtures returning expunged objects caused `DetachedInstanceError` when tests tried to access attributes that weren't pre-loaded.
**Learning:** When using `db.session.expunge(obj)` in a fixture, ensure all required attributes are accessed (loaded) within the session context before expunging, or avoid expunging if the session remains valid for the test duration.
**Prevention:** Always pre-load required attributes before detaching objects from a SQLAlchemy session in test fixtures.
