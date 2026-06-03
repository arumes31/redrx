from flask import Blueprint, request, jsonify, current_app
from werkzeug.security import generate_password_hash
from app.models import db, URL, User
from app.utils import generate_short_code, is_safe_url
from app import limiter, csrf
from app.routes import shortened_links_total
import re
import datetime

api = Blueprint('api', __name__, url_prefix='/api/v1')
csrf.exempt(api)

def get_user_from_api_key():
    api_key = request.headers.get('X-API-KEY')
    if not api_key:
        return None
    return User.query.filter_by(api_key=api_key).first()

def _parse_iso_datetime(dt_str):
    if not dt_str:
        return None
    try:
        return datetime.datetime.fromisoformat(dt_str.replace('Z', '+00:00'))
    except (ValueError, TypeError):
        return None

def _resolve_short_code(custom_code, code_length):
    if custom_code:
        custom_code = custom_code.strip().upper()
        if not (3 <= len(custom_code) <= 20):
            return None, 'custom_code must be between 3 and 20 characters'
        if not re.match(r'^[A-Z0-9_-]+$', custom_code):
            return None, 'custom_code must contain only alphanumeric characters, hyphens, or underscores'
        if URL.query.filter_by(short_code=custom_code).first():
            return None, 'Custom code already taken'
        return custom_code, None
    
    short_code = generate_short_code(code_length)
    while URL.query.filter_by(short_code=short_code).first():
        short_code = generate_short_code(code_length)
    return short_code, None

def _validate_rotate_targets(rotate_targets):
    if rotate_targets is None:
        return None, None
    if not isinstance(rotate_targets, list) or not all(isinstance(u, str) for u in rotate_targets):
        return None, 'rotate_targets must be a list of strings'
    if len(rotate_targets) > 50:
        return None, 'Maximum 50 rotate targets allowed'

    rotate_targets = [u.strip() for u in rotate_targets]
    if not all(is_safe_url(u) for u in rotate_targets):
        return None, 'One or more rotate target URLs are blocked or invalid.'
    return rotate_targets, None

def _validate_input_json():
    data = request.get_json()
    if not isinstance(data, dict):
        return None, 'Request payload must be a JSON object'
    if 'long_url' not in data:
        return None, 'Missing long_url'
    if not isinstance(data.get('long_url'), str):
        return None, 'long_url must be a string'
    return data, None

def _get_expiry_date(data):
    try:
        expiry_hours = int(data.get('expiry_hours', current_app.config['EXPIRY_HOURS']))
        if not (0 <= expiry_hours <= 876000):
            return None, 'expiry_hours must be between 0 and 876,000 (100 years)'
    except (ValueError, TypeError):
        return None, 'expiry_hours must be an integer'

    if expiry_hours == 0:
        return None, None

    try:
        return datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=expiry_hours), None
    except (OverflowError, OSError):
         return None, 'expiry_hours results in a date that is out of range'

def _get_scheduling_dates(data):
    s_str, e_str = data.get('start_at'), data.get('end_at')
    if (s_str and not isinstance(s_str, str)) or (e_str and not isinstance(e_str, str)):
        return None, None, 'scheduling dates must be strings (ISO 8601)'

    start_at = _parse_iso_datetime(s_str)
    if s_str and not start_at:
        return None, None, 'Invalid start_at format. Use ISO 8601'

    end_at = _parse_iso_datetime(e_str)
    if e_str and not end_at:
        return None, None, 'Invalid end_at format. Use ISO 8601'

    if start_at and end_at and end_at <= start_at:
        return None, None, 'Invalid scheduling window: end_at must be after start_at'

    return start_at, end_at, None

def _get_code_length(data):
    try:
        code_length = int(data.get('code_length', current_app.config['SHORT_CODE_LENGTH']))
        if not (3 <= code_length <= 20):
            return None, 'code_length must be between 3 and 20'
        return code_length, None
    except (ValueError, TypeError):
        return None, 'code_length must be an integer'

@api.route('/shorten', methods=['POST'])
@limiter.limit("60 per minute")
def shorten():
    user = get_user_from_api_key()
    if not user:
        return jsonify({'error': 'Valid API Key required. Access denied.'}), 401

    data, error = _validate_input_json()
    if error:
        return jsonify({'error': error}), 400

    long_url = data['long_url'].strip()
    if not is_safe_url(long_url):
        return jsonify({'error': 'Destination URL is blocked'}), 403

    custom_code = data.get('custom_code')
    if custom_code is not None and not isinstance(custom_code, str):
        return jsonify({'error': 'custom_code must be a string'}), 400

    code_length, error = _get_code_length(data)
    if error:
        return jsonify({'error': error}), 400

    short_code, error = _resolve_short_code(custom_code, code_length)
    if error:
        status_code = 409 if 'taken' in error.lower() else 400
        return jsonify({'error': error}), status_code

    expires_at, error = _get_expiry_date(data)
    if error:
        return jsonify({'error': error}), 400

    start_at, end_at, error = _get_scheduling_dates(data)
    if error:
        return jsonify({'error': error}), 400

    rotate_targets, error = _validate_rotate_targets(data.get('rotate_targets'))
    if error:
        status_code = 403 if 'blocked' in error.lower() else 400
        return jsonify({'error': error}), status_code

    password = data.get('password')
    new_url = URL(
        user_id=user.id if user else None,
        short_code=short_code,
        long_url=long_url,
        rotate_targets=rotate_targets,
        password_hash=generate_password_hash(password) if password else None,
        preview_mode=data.get('preview_mode', True),
        stats_enabled=data.get('stats_enabled', True),
        expires_at=expires_at,
        start_at=start_at,
        end_at=end_at
    )
    db.session.add(new_url)
    db.session.commit()
    shortened_links_total.inc()

    return jsonify({
        'short_code': short_code,
        'short_url': f"https://{current_app.config['BASE_DOMAIN']}/{short_code}",
        'long_url': long_url,
        'rotate_targets': rotate_targets,
        'expires_at': expires_at.isoformat() if expires_at else None,
        'start_at': start_at.isoformat() if start_at else None,
        'end_at': end_at.isoformat() if end_at else None,
        'password_protected': bool(password),
        'preview_mode': new_url.preview_mode,
        'stats_enabled': new_url.stats_enabled
    }), 201

@api.route('/<short_code>', methods=['GET'])
def get_url_info(short_code):
    user = get_user_from_api_key()
    if not user:
        return jsonify({'error': 'Valid API Key required. Access denied.'}), 401

    url_entry = URL.query.filter_by(short_code=short_code.upper()).first()
    if not url_entry:
        return jsonify({'error': 'URL not found'}), 404

    return jsonify({
        'short_code': url_entry.short_code,
        'long_url': url_entry.long_url,
        'clicks_count': url_entry.clicks_count,
        'created_at': url_entry.created_at.isoformat(),
        'expires_at': url_entry.expires_at.isoformat() if url_entry.expires_at else None,
        'active': url_entry.is_active()
    })
