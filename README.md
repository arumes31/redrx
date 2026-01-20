# Redirx 🚀

Redirx is a modern, feature-rich URL shortener built with Python (Flask), PostgreSQL, and SQLAlchemy. It features a sleek dark UI with interactive background animations and robust security features.

## ✨ Features

- **Custom Short Codes:** Create memorable links.
- **A/B Testing & Rotation:** Rotate between multiple destination URLs for a single short code.
- **Password Protection:** Secure your links with a password.
- **Expiration & Scheduling:** Set start/end times or automatic expiry in hours.
- **QR Code Generation:** Customizable QR codes with color options and logo support.
- **Bulk Upload:** Shorten hundreds of links at once via CSV.
- **Advanced Stats:** Track click counts and link status.
- **Docker Ready:** Fully containerized with PostgreSQL support.

## 🛠 Tech Stack

- **Backend:** Flask, Flask-SQLAlchemy (ORM), Flask-WTF (Forms)
- **Database:** PostgreSQL (default in Docker), SQLite (fallback)
- **Frontend:** HTML5, CSS3 (Bootstrap 5), JavaScript (Canvas API for animations)
- **Deployment:** Docker, Docker Compose, GitHub Actions

## 🚀 Quick Start (Docker)

The fastest way to get started is using Docker Compose:

1. Clone the repository.
2. Run:
   ```bash
   docker-compose up --build
   ```
3. Open `http://localhost:5000` in your browser.

## 🔧 Local Development

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

### Shorten URL
**Endpoint:** `POST /api/v1/shorten`

**Body:**
```json
{
  "long_url": "https://example.com/my-long-link",
  "custom_code": "optional-custom-code",
  "expiry_hours": 24
}
```

**Response:**
```json
{
  "short_code": "ABC123",
  "short_url": "https://short.example.com/ABC123",
  "long_url": "https://example.com/my-long-link",
  "expires_at": "2024-01-02T12:00:00+00:00"
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