from flask import Blueprint, request, jsonify, current_app
from werkzeug.security import generate_password_hash
from app.models import db, URL, User
from app.utils import generate_short_code, is_safe_url
from app import limiter, csrf
from app.routes import shortened_links_total # Import the custom counter
import datetime
import re

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
        return False # Use False to indicate error since None is a valid "not provided" result

def _resolve_short_code(custom_code, code_length):
    if custom_code:
        if URL.query.filter_by(short_code=custom_code).first():
            return None, jsonify({'error': 'Custom code already taken'}), 409
        return custom_code, None, None
    
    short_code = generate_short_code(code_length)
    while URL.query.filter_by(short_code=short_code).first():
        short_code = generate_short_code(code_length)
    return short_code, None, None

def _validate_rotate_targets(rotate_targets):
    if rotate_targets is None:
        return None, None, None
    if not isinstance(rotate_targets, list) or not all(isinstance(u, str) for u in rotate_targets):
        return None, jsonify({'error': 'rotate_targets must be a list of strings'}), 400
    if len(rotate_targets) > 50:
        return None, jsonify({'error': 'Maximum 50 rotate targets allowed'}), 400

    rotate_targets = [u.strip() for u in rotate_targets]
    if not all(is_safe_url(u) for u in rotate_targets):
        return None, jsonify({'error': 'One or more rotate target URLs are blocked or invalid.'}), 403
    return rotate_targets, None, None

def _validate_custom_code(custom_code):
    if not isinstance(custom_code, str):
        return None, jsonify({'error': 'custom_code must be a string'}), 400
    custom_code = custom_code.strip().upper()
    if len(custom_code) < 3 or len(custom_code) > 20:
        return None, jsonify({'error': 'custom_code must between 3 and 20 characters'}), 400
    if not re.match(r'^[A-Z0-9_-]+$', custom_code):
        return None, jsonify({'error': 'custom_code must contain only alphanumeric characters, hyphens, or underscores'}), 400
    return custom_code, None, None

def _validate_basic_params(data):
    long_url = data.get('long_url')
    if not isinstance(long_url, str):
        return None, jsonify({'error': 'long_url is required and must be a string'}), 400
    
    long_url = long_url.strip()
    if not is_safe_url(long_url):
        return None, jsonify({'error': 'Destination URL is blocked'}), 403

    custom_code = data.get('custom_code')
    if custom_code is not None:
        custom_code, err, status = _validate_custom_code(custom_code)
        if err: return None, err, status

    try:
        code_length = int(data.get('code_length', current_app.config['SHORT_CODE_LENGTH']))
    except (ValueError, TypeError):
        return None, jsonify({'error': 'code_length must be an integer'}), 400
    if code_length < 3 or code_length > 20:
        return None, jsonify({'error': 'code_length must be between 3 and 20'}), 400

    return (long_url, custom_code, code_length), None, None

def _get_expiry_date(hours):
    if hours == 0:
        return None, None
    try:
        return datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=hours), None
    except (OverflowError, OSError):
        return None, jsonify({'error': 'expiry_hours results in a date that is out of range'})

def _validate_scheduling(start_at, end_at):
    if start_at is False or end_at is False:
        return jsonify({'error': 'Invalid date format. Use ISO 8601'}), 400
    if start_at and end_at and end_at <= start_at:
        return jsonify({'error': 'Invalid scheduling window: end_at must be after start_at'}), 400
    return None, None

def _process_url_timestamps(data):
    try:
        expiry_hours = int(data.get('expiry_hours', current_app.config['EXPIRY_HOURS']))
    except (ValueError, TypeError):
        return None, jsonify({'error': 'expiry_hours must be an integer'}), 400
    if expiry_hours < 0 or expiry_hours > 876000:
        return None, jsonify({'error': 'expiry_hours must be between 0 and 876,000 (100 years)'}), 400

    expires_at, err = _get_expiry_date(expiry_hours)
    if err: return None, err, 400

    start_at = _parse_iso_datetime(data.get('start_at'))
    end_at = _parse_iso_datetime(data.get('end_at'))

    err, status = _validate_scheduling(start_at, end_at)
    if err: return None, err, status

    return (expires_at, start_at, end_at), None, None

def _create_new_url(user, short_code, long_url, rotate, data, expires_at, start_at, end_at):
    password = data.get('password')
    new_url = URL(
        user_id=user.id, short_code=short_code, long_url=long_url,
        rotate_targets=rotate,
        password_hash=generate_password_hash(password) if password else None,
        preview_mode=data.get('preview_mode', True),
        stats_enabled=data.get('stats_enabled', True),
        expires_at=expires_at, start_at=start_at, end_at=end_at
    )
    db.session.add(new_url)
    db.session.commit()
    shortened_links_total.inc()
    return new_url

def _build_shorten_response(short_code, long_url, rotate, expires_at, start_at, end_at, password, new_url):
    return jsonify({
        'short_code': short_code,
        'short_url': f"https://{current_app.config['BASE_DOMAIN']}/{short_code}",
        'long_url': long_url,
        'rotate_targets': rotate,
        'expires_at': expires_at.isoformat() if expires_at else None,
        'start_at': start_at.isoformat() if start_at else None,
        'end_at': end_at.isoformat() if end_at else None,
        'password_protected': bool(password),
        'preview_mode': new_url.preview_mode,
        'stats_enabled': new_url.stats_enabled
    }), 201

@api.route('/shorten', methods=['POST'])
@limiter.limit("60 per minute") # Higher limit for API
def shorten():
    user = get_user_from_api_key()
    if not user:
        return jsonify({'error': 'Valid API Key required. Access denied.'}), 401

    data = request.get_json()
    if not isinstance(data, dict):
        return jsonify({'error': 'Request payload must be a JSON object'}), 400

    basic, err, status = _validate_basic_params(data)
    if err: return err, status
    long_url, custom_code, code_length = basic

    times, err, status = _process_url_timestamps(data)
    if err: return err, status
    expires_at, start_at, end_at = times

    short_code, err, status = _resolve_short_code(custom_code, code_length)
    if err: return err, status

    rotate, err, status = _validate_rotate_targets(data.get('rotate_targets'))
    if err: return err, status

    new_url = _create_new_url(user, short_code, long_url, rotate, data, expires_at, start_at, end_at)

    return _build_shorten_response(short_code, long_url, rotate, expires_at, start_at, end_at, data.get('password'), new_url)

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
