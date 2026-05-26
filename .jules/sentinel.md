## 2026-03-23 - Prevent Malicious URL Schemes in Link Shortener
**Vulnerability:** The link shortening API allowed shortening of non-HTTP(S) URLs like `javascript:` and `data:` schemes, leading to Stored XSS.
**Learning:** Even if frontend forms (like WTForms) use URL validators, the API layer must independently strictly validate the URL scheme (allowlisting `http` and `https`) to prevent malicious payloads from bypassing the UI.
**Prevention:** Always enforce a strict scheme allowlist (`['http', 'https']`) on user-provided URLs at the backend/API level before accepting and storing them as redirects.
## 2026-05-26 - Optimize Environment Variable Parsing in High-Frequency Security Functions
**Issue:** Parsing environment variables (like ) inside high-frequency utility functions (like ) caused redundant string processing and  calls.
**Learning:** Cache configuration values derived from environment variables at the module level or during app initialization to improve performance. For domain-based security checks, prefer suffix matching (e.g.,  loop with ) over simple substring matching () to avoid false positives and over-blocking.
**Prevention:** Implement lazy caching for environment variable parsing in performance-critical code paths.
## 2026-05-26 - Optimize Environment Variable Parsing in High-Frequency Security Functions
**Issue:** Parsing environment variables (like BLOCKED_DOMAINS) inside high-frequency utility functions (like is_safe_url) caused redundant string processing and os.environ.get calls.
**Learning:** Cache configuration values derived from environment variables at the module level or during app initialization to improve performance. For domain-based security checks, prefer suffix matching (e.g., while loop with find('.')) over simple substring matching (in) to avoid false positives and over-blocking.
**Prevention:** Implement lazy caching for environment variable parsing in performance-critical code paths.
