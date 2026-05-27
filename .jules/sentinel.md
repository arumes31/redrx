## 2026-03-23 - Prevent Malicious URL Schemes in Link Shortener
**Vulnerability:** The link shortening API allowed shortening of non-HTTP(S) URLs like `javascript:` and `data:` schemes, leading to Stored XSS.
**Learning:** Even if frontend forms (like WTForms) use URL validators, the API layer must independently strictly validate the URL scheme (allowlisting `http` and `https`) to prevent malicious payloads from bypassing the UI.
**Prevention:** Always enforce a strict scheme allowlist (`['http', 'https']`) on user-provided URLs at the backend/API level before accepting and storing them as redirects.
## 2026-03-23 - Prevent Open Redirect in Login
**Vulnerability:** The login route directly used the `next` query parameter in a `redirect()` call without validation, allowing attackers to redirect users to malicious external domains after a successful login.
**Learning:** Redirect targets from user input must always be validated. For internal redirects, ensure the URL starts with a single `/` and not `//` (protocol-relative) to prevent redirects to external domains.
**Prevention:** Use a helper like `is_safe_redirect_url` to validate all redirect targets that originate from request parameters.
