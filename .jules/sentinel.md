## 2026-03-23 - Prevent Malicious URL Schemes in Link Shortener
**Vulnerability:** The link shortening API allowed shortening of non-HTTP(S) URLs like `javascript:` and `data:` schemes, leading to Stored XSS.
**Learning:** Even if frontend forms (like WTForms) use URL validators, the API layer must independently strictly validate the URL scheme (allowlisting `http` and `https`) to prevent malicious payloads from bypassing the UI.
**Prevention:** Always enforce a strict scheme allowlist (`['http', 'https']`) on user-provided URLs at the backend/API level before accepting and storing them as redirects.
## 2026-03-23 - CSV Injection in Links Export
**Vulnerability:** User-controlled fields like `long_url` and `short_code` were exported to CSV without sanitization, allowing spreadsheet formula injection (e.g., payloads starting with `=`, `+`, `-`, or `@`).
**Learning:** CSV exports containing user-generated content must sanitize cells that begin with formula-triggering characters. Prepending a single quote (`'`) is an effective way to force spreadsheet software to treat the cell content as literal text.
**Prevention:** Implement a universal `sanitize_csv_field` helper and apply it to all user-influenced string fields in CSV generation logic.
