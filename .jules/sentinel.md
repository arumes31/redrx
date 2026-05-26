## 2026-03-23 - Prevent Malicious URL Schemes in Link Shortener
**Vulnerability:** The link shortening API allowed shortening of non-HTTP(S) URLs like `javascript:` and `data:` schemes, leading to Stored XSS.
**Learning:** Even if frontend forms (like WTForms) use URL validators, the API layer must independently strictly validate the URL scheme (allowlisting `http` and `https`) to prevent malicious payloads from bypassing the UI.
**Prevention:** Always enforce a strict scheme allowlist (`['http', 'https']`) on user-provided URLs at the backend/API level before accepting and storing them as redirects.

## 2026-05-26 - Robust GeoIP Local IP and Invalid Input Handling
**Issue:** GeoIP lookups were using simplistic string matching for local network detection, missing IPv6 and non-standard private ranges. It also lacked validation for malformed IP strings.
**Learning:** Using the `ipaddress` module provides a standard and robust way to detect all private and loopback IP ranges (IPv4 and IPv6). Explicitly catching `ValueError` during IP parsing prevents 500 errors when malformed strings reach the GeoIP logic.
**Prevention:** Use `ipaddress.ip_address(ip).is_private` or `is_loopback` for reliable local IP detection and wrap IP parsing in try-except blocks.
