<p align="center">
  <img src="internal/web/static/img/logo.png" alt="Redrx Logo" width="120" />
</p>

<h1 align="center">Redrx</h1>

<p align="center">
  A modern, high-performance, and feature-rich self-hosted URL shortener written in Go, backed by PostgreSQL or SQLite with optional Redis. It features a stunning dark UI with interactive animations, robust security protocols, and real-time geographical analytics — shipped as a single static binary.
</p>

<p align="center">
  <a href="https://redrx.eu"><b>Live Demo: redrx.eu</b></a>
</p>

<p align="center">
  <img src="https://img.shields.io/github/actions/workflow/status/arumes31/redirx/docker-build.yml?branch=main&style=for-the-badge&logo=github&color=vibrant" alt="Build Status" />
  <img src="https://img.shields.io/badge/go-1.26-00ADD8?style=for-the-badge&logo=go" alt="Go Version" />
  <img src="https://img.shields.io/badge/security-gosec%20%7C%20govulncheck-yellow?style=for-the-badge&logo=securityscorecard" alt="Security Scanning" />
  <img src="https://img.shields.io/badge/dependencies-up%20to%20date-brightgreen?style=for-the-badge&logo=dependabot" alt="Dependabot" />
  <img src="https://img.shields.io/badge/license-MIT-green?style=for-the-badge" alt="License MIT" />
</p>

---

## 📖 Table of Contents

1. [✨ Features](#-features)
2. [🏗️ Redirection & Security Flow](#️-redirection--security-flow)
3. [🛠️ Tech Stack](#️-tech-stack)
4. [🚀 Quick Start (Docker)](#-quick-start-docker)
5. [💻 Local Development Setup](#-local-development-setup)
6. [🔧 Configuration & Environment Variables](#-configuration--environment-variables)
7. [🔌 REST API Documentation](#-rest-api-documentation)
8. [🛡️ Security and Hardening](#️-security-and-hardening)

---

## ✨ Features

*   🔗 **Custom Short Codes:** Fully customized or auto-generated, readable base62 short keys.
*   🔄 **Rotational Redirects:** Rotate destination traffic between multiple targets using a single short link (perfect for A/B testing or server balancing).
*   🔒 **Password Protection:** Seal individual short links with strong cryptographically-validated access passwords.
*   📅 **Scheduling & Expiration:** Set strict validity windows with `start_at` and `end_at` parameters, or automatic time-to-live (TTL) limits.
*   🎨 **Interactive QR Codes:** Auto-generate customizable SVG/PNG vector QR codes with fully custom colors targeting the short URL directly.
*   📊 **Analytics Dashboard:** Deep visualization on click counters, browser types, platforms, and real-time country detection (powered by local MaxMind GeoIP).
*   🚨 **Phishing Deterrent:** Dual-stage safety verification: cross-checks domain creation against real-time phishing databases with automated malicious link removal.
*   ⚙️ **Access Controls:** Toggle configurations to allow/restrict public registrations or anonymous short link creation.
*   📦 **Single Binary:** Templates and static assets are embedded, so deployment is one ~25 MB static binary with no runtime, interpreter, or asset directory to ship alongside it.

---

## 🏗️ Redirection & Security Flow

Every link request undergoes security screening and database optimization before resolving:

```mermaid
graph TD
    A[User requests short code /ABC123] --> B{Code exists?}
    B -- No --> C[404 Not Found]
    B -- Yes --> D{Enabled and within its schedule?}
    D -- Disabled / expired / outside window --> E[410 Gone]
    D -- Active --> F{Password protected?}
    F -- Yes, not yet unlocked --> G[Prompt for the link password]
    G -- Invalid --> G
    G -- Valid --> H{Device-specific target?}
    F -- No --> H
    H -- iOS / Android --> I[Use the matching device URL]
    H -- Neither --> J{Rotation targets set?}
    J -- Yes --> K[Pick a rotation target at random]
    J -- No --> L[Use the main URL]
    I --> M{Destination on the blocklist?}
    K --> M
    L --> M
    M -- Blocked --> N[403 Forbidden]
    M -- Safe --> O[Record click: country, browser, platform, referrer]
    O --> P{Preview mode?}
    P -- Yes --> Q[Preview page: confirm before leaving]
    P -- No --> R[Interstitial with a 5s countdown, then navigate]
```

The destination is re-checked against the blocklist on every request, not only at creation time, so a link whose target is added to a phishing feed later stops resolving immediately.

---

## 🛠️ Tech Stack

*   **Core Backend:** Go 1.26, standard-library `net/http` routing and `html/template` rendering — no web framework
*   **Data Processing:** PostgreSQL via `pgx` (robust relational store), Redis (shared rate limiting & geo cache), SQLite via the pure-Go `modernc.org/sqlite` driver (resilient local fallback, no cgo)
*   **Geo-Location Engine:** MaxMind GeoLite2 country mapping with a Redis-backed lookup cache
*   **Real-time Metrics:** Integrated Prometheus endpoint handler on `/metrics`
*   **Modern Frontend:** HTML5, CSS3 (Bootstrap 5 Dark Mode theme), custom Canvas API backdrop animations, all embedded into the binary

### Project Layout

```
cmd/redrx/            Entrypoint, logging, background blocklist refresh
internal/config/      Environment configuration
internal/store/       Database access; schema matches the previous SQLAlchemy models
internal/security/    Werkzeug-compatible password hashing
internal/session/     Signed cookie sessions and CSRF tokens
internal/ratelimit/   Flask-Limiter-compatible limit parsing, memory and Redis backends
internal/safety/      Blocked-domain and phishing-feed enforcement
internal/geo/         MaxMind lookups and IP anonymisation
internal/qr/          QR rendering with colours and logo overlay
internal/shortcode/   Short code generation and validation
internal/web/         HTTP handlers, middleware, templates and static assets
```

---

## 🚀 Quick Start (Docker)

Ensure safe execution by loading our production-ready image verified with strict security hashes:

### Using GHCR Image (Recommended)

1. Use `docker-compose.ghcr.yml` as your template:
   ```yaml
   services:
     app:
       image: ghcr.io/arumes31/redrx:latest
       ports:
         - "5000:5000"
       environment:
         - SECRET_KEY=your-production-cryptographic-secret
         - DATABASE_URL=postgresql://redrx:securepassword@db:5432/redrx_db
         - BASE_DOMAIN=short.yourdomain.com
   ```
2. Start the stack:
   ```bash
   docker-compose up -d
   ```

### Local Build
```bash
docker-compose up --build
```
The application will boot and expose itself at `http://localhost:5000`.

---

## 💻 Local Development Setup

Requires Go 1.26.5 or newer, the minimum declared by the `go` directive in `go.mod`. Dependencies are pinned in `go.mod` and checksum-verified through `go.sum`.

### 1. Build

```bash
go mod download
go build ./cmd/redrx
```

### 2. Run

The server defaults to SQLite at `db/shortener.db`, so no external services are needed for development:

```bash
# Windows PowerShell
$env:SECRET_KEY = "dev-secret"
$env:REDRX_DEBUG = "true"
$env:BASE_DOMAIN = "localhost:5000"
go run ./cmd/redrx
```

```bash
# Linux / macOS
SECRET_KEY=dev-secret REDRX_DEBUG=true BASE_DOMAIN=localhost:5000 go run ./cmd/redrx
```

The application boots at `http://localhost:5000`. Setting `REDRX_DEBUG` (or the legacy `FLASK_DEBUG`) relaxes the canonical-domain redirect, allows a fallback `SECRET_KEY`, and drops the `Secure` flag from session cookies so plain HTTP works locally.

### 3. Tests

```bash
go test ./...              # full suite
go test -race ./...        # with the race detector
go vet ./...               # static analysis
```

The suite includes a committed SQLite fixture written by the previous Python application (`internal/store/testdata/legacy_python.db`). Tests read and write it to prove that existing databases, password hashes and timestamp formats remain compatible.

---

## 🔧 Configuration & Environment Variables

| Category | Variable | Default | Description |
|----------|----------|---------|-------------|
| **Core** | `SECRET_KEY` | - | Strong cryptographic key for session signing and hashing. Enforced in production. |
| **Domain** | `BASE_DOMAIN` | `short.example.com` | Base host string used when formatting shortened URLs. |
| **GeoIP** | `MAXMIND_LICENSE_KEY` | - | Required to download the GeoIP dataset and update in background. |
| **Phishing** | `ENABLE_PHISHING_CHECK` | `true` | Enables domain protection against real-time blacklists. |
| **Phishing** | `ENABLE_AUTO_REMOVE_PHISHING` | `false` | Automatically removes links that redirect to verified phishing domains. |
| **Database** | `DATABASE_URL` | - | PostgreSQL URI (e.g. `postgresql://user:pass@host:5432/db`). Defaults to SQLite local file fallback. |
| **Limits** | `RATELIMIT_STORAGE_URL` | `redis://redis:6379` | Rate limiting backend. Can fall back to local storage `memory://` in dev. |
| **Access** | `DISABLE_ANONYMOUS_CREATE` | `false` | When true, only authenticated users can shorten links. |
| **Access** | `DISABLE_REGISTRATION` | `false` | When true, public registration routes are disabled. |
| **Proxy** | `TRUSTED_PROXIES` | - | Peers whose `X-Forwarded-*` / `CF-*` headers are believed (IPs or CIDRs, comma separated; `*` for any). Empty means the headers are ignored. |
| **Proxy** | `USE_CLOUDFLARE` | `false` | Trust `CF-Connecting-IP` from a trusted proxy. Required for correct client IPs behind Cloudflare. |
| **Server** | `LISTEN_ADDR` | `:5000` | Address the HTTP server binds to. |
| **Server** | `REDRX_DEBUG` | `false` | Development mode. Relaxes the canonical-domain redirect and allows a fallback `SECRET_KEY`. |

### Running behind a reverse proxy

Rate limiting, analytics and the abuse controls all key off the client IP, so
redrx has to be told who is allowed to tell it what that IP is. **By default it
trusts nobody and uses the peer address**, which is correct for a directly
exposed service: a client that can set its own `X-Forwarded-For` can otherwise
pick its own rate-limit bucket and bypass every limit, including the one in
front of login.

For the common **Cloudflare → nginx → redrx** chain:

```env
TRUSTED_PROXIES=172.18.0.0/16   # nginx, as redrx sees it
USE_CLOUDFLARE=true
```

`USE_CLOUDFLARE` matters more than it looks. nginx appends the address it
received from to `X-Forwarded-For`, so the header arrives as
`<visitor>, <cloudflare-edge>` — the *last* entry is a Cloudflare server, not
your user. `CF-Connecting-IP` names the real visitor, so redrx prefers it. The
alternative is to add Cloudflare's published ranges to `TRUSTED_PROXIES`, which
lets redrx walk back past them; without one of the two, every visitor on the
internet shares the handful of buckets belonging to the edge servers.

Rate limits use the same syntax as before (`"200 per day;50 per hour"`, `"10 per minute"`, `"5/hour"`). `RATELIMIT_STORAGE_URL` accepts `memory://` or a `redis://` URL; when Redis is configured it also backs the GeoIP lookup cache. If Redis is unreachable at boot the service logs a warning and falls back to in-memory limiting rather than refusing to start.

---

## 🔌 REST API Documentation

For the full detailed documentation including parameter tables and system metrics, visit the `/api-docs` page on your running Redrx instance.

### Authentication
Include your personal API key (available inside your user Profile menu) in request headers:
```http
X-API-KEY: your_api_key_here
```

### Shorten a URL
`POST /api/v1/shorten`

**Payload:**
```json
{
  "long_url": "https://example.com/my-long-link",
  "custom_code": "my-code",
  "code_length": 6,
  "preview_mode": true,
  "stats_enabled": true,
  "rotate_targets": ["https://alt1.com", "https://alt2.com"],
  "ios_target_url": "https://apps.apple.com/app/id123",
  "android_target_url": "https://play.google.com/store/apps/details?id=com.example",
  "password": "secret-password",
  "expiry_hours": 24,
  "start_at": "2026-06-05T22:00:00Z",
  "end_at": "2026-06-30T23:59:59Z"
}
```

**Response (201 Created):**
```json
{
  "short_code": "my-code",
  "short_url": "https://short.example.com/my-code",
  "long_url": "https://example.com/my-long-link",
  "rotate_targets": ["https://alt1.com", "https://alt2.com"],
  "ios_target_url": "https://apps.apple.com/app/id123",
  "android_target_url": "https://play.google.com/store/apps/details?id=com.example",
  "expires_at": "2026-06-06T22:00:00+00:00",
  "start_at": "2026-06-05T22:00:00+00:00",
  "end_at": "2026-06-30T23:59:59+00:00",
  "password_protected": true,
  "preview_mode": true,
  "stats_enabled": true
}
```

### Query Link Information
`GET /api/v1/<short_code>`

**Response (200 OK):**
```json
{
  "short_code": "my-code",
  "short_url": "https://short.example.com/my-code",
  "long_url": "https://example.com/my-long-link",
  "rotate_targets": ["https://alt1.com", "https://alt2.com"],
  "ios_target_url": "https://apps.apple.com/app/id123",
  "android_target_url": "https://play.google.com/store/apps/details?id=com.example",
  "preview_mode": true,
  "stats_enabled": true,
  "clicks_count": 42,
  "clicks": 42,
  "created_at": "2026-06-05T22:00:00+00:00",
  "expires_at": "2026-06-06T22:00:00+00:00",
  "start_at": "2026-06-05T22:00:00+00:00",
  "end_at": "2026-06-30T23:59:59+00:00",
  "active": true
}
```

---

## 🛡️ Security and Hardening

- **SAST and Vulnerability Scanning:** `gosec`, `govulncheck` and CodeQL run on every push and nightly.
- **Supply Chain:** `go.sum` pins a checksum for every dependency; CI runs `go mod verify`, and Trivy and Gitleaks scan the tree.
- **Fail-Closed Blocklist:** If phishing checking is enabled but the blocklist cannot be read and nothing is cached, every destination is rejected rather than silently allowing traffic past an unenforced filter.
- **Session Integrity:** Cookies are HMAC-SHA256 signed, `HttpOnly`, `SameSite=Lax`, and `Secure` outside debug. CSRF tokens are per-session, constant-time compared, and rotated on login.
- **Privacy:** Client IPs are truncated to two octets (IPv4) or two groups (IPv6) before being stored, and `ANONYMIZE_LOGS` masks them in log output.
- **Container:** Runs as a non-root user from a minimal Alpine base. The Go toolchain stays in the build stage, so the runtime image carries only the static binary and its CA certificates — no compiler and no language runtime.
- **Dependabot Enforced:** Automatic dependency tracking for patches and security updates.

---

## 📄 License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
