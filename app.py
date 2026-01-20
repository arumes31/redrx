# app.py (updated)
import os
import sqlite3
import uuid
import datetime
import io
import base64
import csv
import json
from urllib.parse import urlparse, urlunparse, parse_qs, urlencode
from werkzeug.security import generate_password_hash, check_password_hash
from flask import Flask, render_template, request, redirect, abort, url_for, send_file, flash, session
import qrcode
from PIL import Image

app = Flask(__name__)
app.secret_key = os.urandom(24)  # For session/flash

# Database setup
DB_PATH = '/db/shortener.db'

def init_db():
    conn = sqlite3.connect(DB_PATH)
    c = conn.cursor()
    c.execute('''
        CREATE TABLE IF NOT EXISTS urls (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            short_code TEXT UNIQUE NOT NULL,
            long_url TEXT NOT NULL,
            ab_urls TEXT,  -- JSON list of alternate URLs
            password_hash TEXT,
            clicks INTEGER DEFAULT 0,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            expires_at TIMESTAMP,
            start_at TIMESTAMP,
            end_at TIMESTAMP
        )
    ''')
    conn.commit()
    conn.close()

init_db()

BASE_DOMAIN = os.getenv('BASE_DOMAIN', 'short.example.com')
EXPIRY_HOURS = int(os.getenv('EXPIRY_HOURS', 24))
SHORT_CODE_LENGTH = int(os.getenv('SHORT_CODE_LENGTH', 6))
DEFAULT_QR_COLOR = os.getenv('DEFAULT_QR_COLOR', 'black')
DEFAULT_QR_BG = os.getenv('DEFAULT_QR_BACKGROUND', 'white')

def generate_short_code():
    return str(uuid.uuid4())[:SHORT_CODE_LENGTH].upper()

def is_active(start_at, end_at, expires_at):
    now = datetime.datetime.now()
    if start_at and now < datetime.datetime.fromisoformat(start_at):
        return False
    if end_at and now > datetime.datetime.fromisoformat(end_at):
        return False
    if expires_at and now > datetime.datetime.fromisoformat(expires_at):
        return False
    return True

def get_url(short_code):
    conn = sqlite3.connect(DB_PATH)
    c = conn.cursor()
    c.execute('SELECT long_url, ab_urls, password_hash, expires_at, start_at, end_at, clicks FROM urls WHERE short_code = ?', (short_code,))
    row = c.fetchone()
    conn.close()
    if row:
        return {
            'long_url': row[0],
            'ab_urls': json.loads(row[1]) if row[1] else None,
            'password_hash': row[2],
            'expires_at': row[3],
            'start_at': row[4],
            'end_at': row[5],
            'clicks': row[6]
        }
    return None

def select_ab_url(ab_urls):
    if not ab_urls:
        return None
    return ab_urls[hash(str(datetime.datetime.now())) % len(ab_urls)]  # Simple round-robin via hash

def generate_qr(short_url, color=DEFAULT_QR_COLOR, bg=DEFAULT_QR_BG, logo_img=None):
    qr = qrcode.QRCode(version=1, box_size=10, border=5)
    qr.add_data(short_url)
    qr.make(fit=True)
    img = qr.make_image(fill_color=color, back_color=bg)
    if logo_img:
        logo_img.thumbnail((img.size[0]//5, img.size[1]//5))
        pos = ((img.size[0] - logo_img.size[0]) // 2, (img.size[1] - logo_img.size[1]) // 2)
        img.paste(logo_img, pos)
    img_buffer = io.BytesIO()
    img.save(img_buffer, format='PNG')
    img_buffer.seek(0)
    return img_buffer

@app.route('/', methods=['GET', 'POST'])
def index():
    short_url = None
    qr_data = None
    stats_url = None
    error = None
    if request.method == 'POST':
        is_bulk = 'csv_file' in request.files
        if is_bulk:
            return handle_bulk(request)
        
        long_url = request.form.get('long_url')
        custom_code = request.form.get('custom_code', '').strip().upper()
        ab_urls = request.form.get('ab_urls', '').strip()
        password = request.form.get('password')
        start_date = request.form.get('start_date')
        start_time = request.form.get('start_time')
        end_date = request.form.get('end_date')
        end_time = request.form.get('end_time')
        expiry_hours = request.form.get('expiry_hours')
        disable_expiry = request.form.get('disable_expiry')
        qr_color = request.form.get('qr_color', DEFAULT_QR_COLOR)
        qr_bg = request.form.get('qr_bg', DEFAULT_QR_BG)
        logo_file = request.files.get('logo_file') if request.files else None

        if not long_url:
            error = "Please enter a URL."
        else:
            parsed = urlparse(long_url)
            if not parsed.scheme or not parsed.netloc:
                error = "Invalid URL format."
            else:
                short_code = custom_code if custom_code else generate_short_code()
                ab_list = [u.strip() for u in ab_urls.split(',') if u.strip()] if ab_urls else None
                if ab_list and len(ab_list) > 1:
                    ab_json = json.dumps(ab_list)
                else:
                    ab_json = None
                password_hash = generate_password_hash(password) if password else None
                start_at = None
                if start_date and start_time:
                    start_at = datetime.datetime.strptime(f"{start_date} {start_time}", '%Y-%m-%d %H:%M').isoformat()
                end_at = None
                if end_date and end_time:
                    end_at = datetime.datetime.strptime(f"{end_date} {end_time}", '%Y-%m-%d %H:%M').isoformat()
                expires_at = None
                if not disable_expiry:
                    expiry_delta = datetime.timedelta(hours=int(expiry_hours or EXPIRY_HOURS))
                    expires_at = (datetime.datetime.now() + expiry_delta).isoformat()
                logo_img = None
                if logo_file and logo_file.filename:
                    logo_buffer = io.BytesIO(logo_file.read())
                    try:
                        logo_img = Image.open(logo_buffer)
                    except Exception:
                        flash('Invalid logo image.')
                
                conn = sqlite3.connect(DB_PATH)
                c = conn.cursor()
                try:
                    c.execute(
                        'INSERT INTO urls (short_code, long_url, ab_urls, password_hash, expires_at, start_at, end_at) VALUES (?, ?, ?, ?, ?, ?, ?)',
                        (short_code, long_url, ab_json, password_hash, expires_at, start_at, end_at)
                    )
                    conn.commit()
                    short_url = f"https://{BASE_DOMAIN}/{short_code}"
                    stats_url = f"https://{BASE_DOMAIN}/{short_code}/stats"
                    
                    # Generate QR
                    img_buffer = generate_qr(short_url, qr_color, qr_bg, logo_img)
                    img_buffer.seek(0)  # Ensure position
                    qr_data = base64.b64encode(img_buffer.read()).decode()
                    
                except sqlite3.IntegrityError:
                    error = f"Short code '{short_code}' already exists. Try another."
                finally:
                    conn.close()

    return render_template('index.html', short_url=short_url, qr_data=qr_data, stats_url=stats_url, error=error,
                           default_qr_color=DEFAULT_QR_COLOR, default_qr_bg=DEFAULT_QR_BG, short_code_length=SHORT_CODE_LENGTH)

def handle_bulk(request):
    file = request.files['csv_file']
    if file.filename == '':
        flash('No file selected.')
        return redirect(url_for('index'))
    
    stream = io.StringIO(file.stream.read().decode("UTF8"), newline=None)
    csv_reader = csv.DictReader(stream)
    results = []
    errors = []
    for row in csv_reader:
        long_url = row.get('long_url', '').strip()
        if not long_url:
            errors.append("Missing long_url in row")
            continue
        custom_code = row.get('custom_code', '').strip().upper() or generate_short_code()
        expires_at = (datetime.datetime.now() + datetime.timedelta(hours=EXPIRY_HOURS)).isoformat()
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        try:
            c.execute('INSERT INTO urls (short_code, long_url, expires_at) VALUES (?, ?, ?)', (custom_code, long_url, expires_at))
            conn.commit()
            short_url = f"https://{BASE_DOMAIN}/{custom_code}"
            results.append(short_url)
        except sqlite3.IntegrityError:
            # Regenerate on collision
            custom_code = generate_short_code()
            c.execute('INSERT INTO urls (short_code, long_url, expires_at) VALUES (?, ?, ?)', (custom_code, long_url, expires_at))
            conn.commit()
            short_url = f"https://{BASE_DOMAIN}/{custom_code}"
            results.append(short_url)
        finally:
            conn.close()
    
    flash(f'Processed {len(results)} links. Errors: {len(errors)}')
    return render_template('bulk.html', results=results, errors=errors)

@app.route('/<short_code>')
def redirect_short(short_code):
    url_data = get_url(short_code)
    if not url_data:
        abort(404)

    if not is_active(url_data['start_at'], url_data['end_at'], url_data['expires_at']):
        abort(410)

    if url_data['password_hash']:
        # Simple session-based auth; in prod, use proper login flow
        if 'password' not in session or not check_password_hash(url_data['password_hash'], session.get('password', '')):
            return redirect(url_for('login', short_code=short_code))

    long_url = url_data['long_url']
    if url_data['ab_urls']:
        long_url = select_ab_url(url_data['ab_urls'])

    # Increment clicks
    conn = sqlite3.connect(DB_PATH)
    c = conn.cursor()
    c.execute('UPDATE urls SET clicks = clicks + 1 WHERE short_code = ?', (short_code,))
    conn.commit()
    conn.close()

    return redirect(long_url, code=302)

@app.route('/login/<short_code>', methods=['GET', 'POST'])
def login(short_code):
    if request.method == 'POST':
        password = request.form.get('password')
        url_data = get_url(short_code)
        if url_data and url_data['password_hash'] and check_password_hash(url_data['password_hash'], password):
            session['password'] = password
            return redirect(url_for('redirect_short', short_code=short_code))
        flash('Invalid password.')
    return render_template('login.html', short_code=short_code)

@app.route('/<short_code>/stats')
def stats(short_code):
    url_data = get_url(short_code)
    if not url_data:
        abort(404)
    if not is_active(url_data['start_at'], url_data['end_at'], url_data['expires_at']):
        abort(410)

    short_url = f"https://{BASE_DOMAIN}/{short_code}"
    active = is_active(url_data['start_at'], url_data['end_at'], url_data['expires_at'])
    return render_template('stats.html', short_code=short_code, short_url=short_url, long_url=url_data['long_url'], 
                          ab_urls=url_data['ab_urls'], password_protected=bool(url_data['password_hash']),
                          clicks=url_data['clicks'], start_at=url_data['start_at'], end_at=url_data['end_at'], 
                          expires_at=url_data['expires_at'], active=active, base_domain=BASE_DOMAIN)

@app.route('/<short_code>/qr')
def qr(short_code):
    url_data = get_url(short_code)
    if not url_data:
        abort(404)
    short_url = f"https://{BASE_DOMAIN}/{short_code}"
    img_buffer = generate_qr(short_url)
    return send_file(img_buffer, mimetype='image/png', as_attachment=False, download_name=f'{short_code}.png')

@app.errorhandler(404)
def not_found(error):
    return render_template('404.html'), 404

@app.errorhandler(410)
def gone(error):
    return render_template('410.html'), 410

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=5000, debug=False)