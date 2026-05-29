<p align="center">
  <img src="app/static/img/logo.png" alt="Redrx Logo" width="120" />
</p>

<h1 align="center">Redrx 🚀</h1>

<p align="center">
  A modern, high-performance, and feature-rich self-hosted URL shortener built with Python (Flask), PostgreSQL/SQLite, Redis, and SQLAlchemy. It features a stunning dark UI with interactive animations, robust security protocols, and real-time geographical analytics.
</p>

<p align="center">
  <a href="https://redrx.eu"><b>Live Demo: redrx.eu</b></a>
</p>

<p align="center">
  <img src="https://img.shields.io/github/actions/workflow/status/arumes31/redirx/docker-build.yml?branch=main&style=for-the-badge&logo=github&color=vibrant" alt="Build Status" />
  <img src="https://img.shields.io/badge/python-3.14-blue?style=for-the-badge&logo=python" alt="Python Version" />
  <img src="https://img.shields.io/badge/security-bandit-yellow?style=for-the-badge&logo=securityscorecard" alt="Security Bandit" />
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
*   📦 **Strict Hash Verification:** Production locks are secured with strict SHA-256 integrity verification (`--require-hashes`) for container builds.

---

## 🏗️ Redirection & Security Flow

Every link request undergoes security screening and database optimization before resolving:

```mermaid
graph TD
    A[User requests short code /ABC123] --> B{Phishing Check}
    B -- Is Domain Blocked? --> C[Return 403 Forbidden]
    B -- Safe --> D{Expired / Inactive?}
    D -- Expired/Future Window --> E[Return 404 Not Found]
    D -- Active --> F{Password Protected?}
    F -- Yes --> G[Prompt User for Password]
    G -- Invalid --> G
    G -- Valid --> H{Rotational Redirect?}
    F -- No --> H
    H -- Yes --> I[Resolve Rotated Target]
    H -- No --> J[Resolve Main URL]
    I --> K[Update Analytics: GeoIP / User Agent]
    J --> K
    K --> L[302 Redirect to Target]
```

---

## 🛠️ Tech Stack

*   **Core Backend:** Python 3.14, Flask, SQLAlchemy, Gunicorn (WSGI HTTP server)
*   **Data Processing:** PostgreSQL (robust relational store), Redis (fast rate limiting & session cache), SQLite (resilient local fallback)
*   **Geo-Location Engine:** MaxMind GeoLite2 country mapping with automatic local file update background task
*   **Real-time Metrics:** Integrated Prometheus endpoint handler on `/metrics`
*   **Modern Frontend:** HTML5, CSS3 (Bootstrap 5 Dark Mode theme), custom Canvas API backdrop animations

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

To secure development packages from production builds, dependencies are segregated into human-editable templates and strict hash-verified lock files.

### 1. Structure
*   `requirements.txt`: Master direct runtime dependency source.
*   `requirements.lock.txt`: production locked pins, generated with SHA-256 package signatures (`--require-hashes`).
*   `requirements-dev.txt`: Dev dependencies including pytest.
*   `requirements-dev.lock.txt`: dev-specific lock with full package tree and hashes.

### 2. Environment Installation
Ensure virtual environment activation and run strict hash-verified installation:
```powershell
# Windows PowerShell
python -m venv venv
.\venv\Scripts\Activate.ps1

# Install package set with secure hashes
pip install --require-hashes -r requirements-dev.lock.txt
```

### 3. Execution & Tests
Set your local env to debug/development and launch:
```powershell
# Run the local server
$env:FLASK_DEBUG="true"
python run.py

# Execute full automated test suite (all checks pass)
pytest
```

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

---

## 🔌 REST API Documentation

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
  "long_url": "https://google.com",
  "custom_code": "gg",
  "expiry_hours": 48,
  "password": "optional-access-password",
  "rotate_targets": ["https://google.ca", "https://google.co.uk"],
  "stats_enabled": true
}
```

**Response (201 Created):**
```json
{
  "short_code": "gg",
  "short_url": "https://short.example.com/gg",
  "long_url": "https://google.com",
  "expires_at": "2026-05-31T16:00:00+00:00",
  "password_protected": true,
  "stats_enabled": true,
  "rotate_targets": ["https://google.ca", "https://google.co.uk"]
}
```

### Query Link Information
`GET /api/v1/<short_code>`

**Response (200 OK):**
```json
{
  "short_code": "gg",
  "long_url": "https://google.com",
  "clicks": 184,
  "created_at": "2026-05-29T16:00:00",
  "expires_at": "2026-05-31T16:00:00",
  "active": true
}
```

---

## 🛡️ Security and Hardening

- **Bandit SAST Engine:** Automated static security scans executed continuously.
- **Dependency Isolation:** Separate locks isolate development-only code from the production Gunicorn engine.
- **Lock Verification:** Lock files are cryptographically validated to defend against supply chain attacks.
- **Dependabot Enforced:** Automatic dependency tracking to patches and security updates.

---

## 📄 License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
