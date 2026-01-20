# Redirx Refactoring & Improvement TODO List - COMPLETED

## 1. Project Structure & Configuration
- [x] Create `app/` directory structure (init, static, templates).
- [x] Create `config.py` to handle `os.getenv` variables (Secret key, DB path, etc.).
- [x] Create `run.py` as the new entry point.
- [x] Update `requirements.txt` (Add `Flask-SQLAlchemy`, `Flask-WTF`).
- [x] Added `example.env` template.

## 2. Backend Refactoring (Modularization)
- [x] **Database**: Replace raw SQLite with `Flask-SQLAlchemy` models in `app/models.py`.
- [x] **Forms**: Create `app/forms.py` for URL submission, Login, and Bulk Upload handling.
- [x] **Utils**: Move QR generation, short code logic, and bulk CSV parsing to `app/utils.py`.
- [x] **Routes**: Move all `@app.route` logic to `app/routes.py` (using Blueprints).

## 3. Frontend Rework (HTML/CSS/JS)
- [x] Create `app/templates/base.html` for shared layout (Navbar, Canvas background, Scripts).
- [x] Extract inline CSS to `app/static/css/style.css`.
- [x] Extract Canvas/Particle scripts to `app/static/js/main.js`.
- [x] Update `index.html`, `login.html`, `stats.html` to extend `base.html`.
- [x] Improve UI responsiveness and accessibility.

## 4. Testing & Quality
- [x] Create `tests/` directory.
- [x] Write basic unit tests for URL creation, redirection, and expiration logic.
- [x] Manual verification of QR codes and password protection.

## 5. Cleanup
- [x] Delete original `app.py` (after verifying `run.py`).
- [x] Remove old templates/statics if not used.
