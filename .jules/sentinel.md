## 2026-03-23 - Prevent Malicious URL Schemes in Link Shortener
**Vulnerability:** The link shortening API allowed shortening of non-HTTP(S) URLs like `javascript:` and `data:` schemes, leading to Stored XSS.
**Learning:** Even if frontend forms (like WTForms) use URL validators, the API layer must independently strictly validate the URL scheme (allowlisting `http` and `https`) to prevent malicious payloads from bypassing the UI.
**Prevention:** Always enforce a strict scheme allowlist (`['http', 'https']`) on user-provided URLs at the backend/API level before accepting and storing them as redirects.

## 2026-03-23 - Enforce Static Secret Keys in Production
**Vulnerability:** Use of `os.urandom(24)` as a fallback for `SECRET_KEY` caused session invalidation on every app restart and allowed insecure silent fallbacks if the environment variable was missing.
**Learning:** Production applications should never silently fall back to dynamic or weak secrets. Enforce the presence of a cryptographically secure, static `SECRET_KEY` via environment variables and fail-fast (e.g., raise `RuntimeError`) during configuration loading if it is missing.
**Prevention:** In the configuration class, check for the required secret and raise an exception if it's missing while not in debug mode. Provide a clear error message for operators.
