## 2026-03-23 - Prevent Malicious URL Schemes in Link Shortener
**Vulnerability:** The link shortening API allowed shortening of non-HTTP(S) URLs like `javascript:` and `data:` schemes, leading to Stored XSS.
**Learning:** Even if frontend forms (like WTForms) use URL validators, the API layer must independently strictly validate the URL scheme (allowlisting `http` and `https`) to prevent malicious payloads from bypassing the UI.
**Prevention:** Always enforce a strict scheme allowlist (`['http', 'https']`) on user-provided URLs at the backend/API level before accepting and storing them as redirects.

## 2026-05-28 - Insecure Secret Key Generation Fallback
**Vulnerability:** The application used `os.urandom(24)` as a dynamic fallback for `SECRET_KEY`. This meant session tokens were invalidated every time the app restarted. Furthermore, it allowed the app to run in production without an explicitly set, persistent secret key.
**Learning:** Dynamic generation of `SECRET_KEY` is unsuitable for session persistence across restarts. Production environments should strictly enforce the presence of a static, secure key via environment variables to ensure both security and session stability.
**Prevention:** In the application configuration, check for the presence of `SECRET_KEY`. Raise a `RuntimeError` if it is missing in production. For development/debug modes, use a static, well-known development key to maintain session consistency while signaling that it is not for production use.

## 2026-06-15 - Missing API Validation and Shortcode Collision Testing
**Vulnerability:** The API lacked comprehensive input validation and robust testing for shortcode collision resolution. While the logic existed, it was untested, leaving potential for bugs in edge cases like sequential collisions or malformed inputs.
**Learning:** High coverage is critical for security-sensitive endpoints. Using tools like Radon to identify high cyclomatic complexity can point to areas that need refactoring and more granular testing.
**Prevention:** Implement exhaustive unit tests covering all error response branches (400, 401, 403, 404, 409). Use mocks to simulate shortcode collisions and ensure the system recovers gracefully. Refactor complex routes into smaller, testable helper functions.
