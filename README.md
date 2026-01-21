# Redrx 🚀

Redrx is a modern, feature-rich URL shortener built with Python (Flask), PostgreSQL, and SQLAlchemy. It features a sleek dark UI with interactive background animations and robust security features.

## ✨ Features

- **Custom Short Codes:** Create memorable links.
- **Rotate Targets:** Rotate between multiple destination URLs for a single short code.
- **Password Protection:** Secure your links with a password.
- **Expiration & Scheduling:** Set start/end times or automatic expiry (set to 0 for permanent links).
- **QR Code Generation:** Customizable QR codes with color options and logo support, targeting the shortened URL.
- **Bulk Upload:** Shorten hundreds of links at once via CSV.
- **Advanced Stats:** Track click counts, countries (Local GeoIP), browsers, and platforms.
- **Phishing Protection:** Auto-updating blocked domain lists with optional auto-removal of malicious links.
- **Access Control:** Configurable options to disable anonymous link creation or public registration.
- **Docker Ready:** Fully containerized with PostgreSQL and GeoIP auto-updater.

## 🛠 Tech Stack

- **Backend:** Flask, Flask-SQLAlchemy (ORM), Flask-WTF (Forms)
- **Database:** PostgreSQL (default in Docker), SQLite (fallback)
- **Geo-Location:** MaxMind GeoLite2 (Local mmdb with auto-updater)
- **Frontend:** HTML5, CSS3 (Bootstrap 5), JavaScript (Canvas API for animations)
- **Deployment:** Docker, GHCR, GitHub Actions

## 🚀 Quick Start (Docker)

### Using GHCR Image (Recommended)
Create a `docker-compose.yml` using `docker-compose.ghcr.yml` as a template and run:
```bash
docker-compose up -d
```

### Local Build
1. Clone the repository.
2. Run:
   ```bash
   docker-compose up --build
   ```
3. Open `http://localhost:5000` in your browser.

## 🔧 Configuration (Environment Variables)

| Variable | Default | Description |
|----------|---------|-------------|
| `MAXMIND_LICENSE_KEY` | - | Required for local GeoIP updates. |
| `ENABLE_PHISHING_CHECK` | `true` | Toggle phishing domain blocking. |
| `ENABLE_AUTO_REMOVE_PHISHING` | `false` | Automatically delete links found on phishing lists. |
| `PHISHING_LIST_URLS` | `https://raw.githubusercontent.com/mitchellkrogza/Phishing.Database/master/phishing-domains-ACTIVE.txt` | Comma-separated list of phishing list sources. |
| `DISABLE_ANONYMOUS_CREATE` | `false` | If true, only logged-in users can shorten URLs. |
| `DISABLE_REGISTRATION` | `false` | If true, the registration page is disabled. |
| `USE_CLOUDFLARE` | `false` | Enable support for Cloudflare headers (CF-Connecting-IP, CF-IPCountry). |
| `RATELIMIT_STORAGE_URL` | `memory://` | Storage backend for rate limiting (e.g., `redis://localhost:6379`). |

1. Install dependencies:
   ```bash
   pip install -r requirements.txt
   ```
2. Configure environment variables (see `example.env`).
3. Run the app:
   ```bash
   python run.py
   ```

## 🔌 API Documentation

### Authentication
To use the API, you must provide your personal API key in the request headers. You can find your API key in your user profile/settings after logging in.

**Header:** `X-API-KEY: your_api_key_here`

### Shorten URL
**Endpoint:** `POST /api/v1/shorten`

**Body:**
```json
{
  "long_url": "https://example.com/my-long-link",
  "custom_code": "optional-custom-code",
  "expiry_hours": 24,
  "password": "secret-password",
  "rotate_targets": ["https://alt1.com", "https://alt2.com"],
  "stats_enabled": true,
  "start_at": "2024-01-01T12:00:00Z",
  "end_at": "2024-12-31T12:00:00Z"
}
```

**Response:**
```json
{
  "short_code": "ABC123",
  "short_url": "https://short.example.com/ABC123",
  "long_url": "https://example.com/my-long-link",
  "expires_at": "2024-01-02T12:00:00+00:00",
  "password_protected": true,
  "preview_mode": true,
  "stats_enabled": true,
  "rotate_targets": ["https://alt1.com", "https://alt2.com"]
}
```

### Get URL Info
**Endpoint:** `GET /api/v1/<short_code>`

**Response:**
```json
{
  "short_code": "ABC123",
  "long_url": "https://example.com",
  "clicks": 42,
  "created_at": "2024-01-01T12:00:00",
  "expires_at": "2024-01-02T12:00:00",
  "active": true
}
```

## 🧪 Running Tests

```bash
python -m pytest
```

## 🛡 Security

- Daily security scans via Bandit.
- Docker image builds on every push to `main`.
- Automated dependency updates via Dependabot.