# Redirx Refactoring & Improvement TODO List

## 1. Project Structure & Configuration
- [ ] Create `app/` directory structure (init, static, templates).
- [ ] Create `config.py` to handle `os.getenv` variables (Secret key, DB path, etc.).
- [ ] Create `run.py` as the new entry point.
- [ ] Update `requirements.txt` (Add `Flask-SQLAlchemy`, `Flask-WTF`).

## 2. Backend Refactoring (Modularization)
- [ ] **Database**: Replace raw SQLite with `Flask-SQLAlchemy` models in `app/models.py`.
- [ ] **Forms**: Create `app/forms.py` for URL submission, Login, and Bulk Upload handling.
- [ ] **Utils**: Move QR generation, short code logic, and bulk CSV parsing to `app/utils.py`.
- [ ] **Routes**: Move all `@app.route` logic to `app/routes.py` (using Blueprints).

## 3. Frontend Rework (HTML/CSS/JS)
- [ ] Create `app/templates/base.html` for shared layout (Navbar, Canvas background, Scripts).
- [ ] Extract inline CSS to `app/static/css/style.css`.
- [ ] Extract Canvas/Particle scripts to `app/static/js/background.js`.
- [ ] Update `index.html`, `login.html`, `stats.html` to extend `base.html`.
- [ ] Improve UI responsiveness and accessibility.

## 4. Testing & Quality
- [ ] Create `tests/` directory.
- [ ] Write basic unit tests for URL creation, redirection, and expiration logic.
- [ ] Manual verification of QR codes and password protection.

## 5. Cleanup
- [ ] Delete original `app.py` (after verifying `run.py`).
- [ ] Remove old templates/statics if not used.
