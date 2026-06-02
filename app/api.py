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

def _parse_iso_datetime(dt_str, field_name):
    if dt_str is None:
        return None, None
    if not isinstance(dt_str, str):
        return None, (jsonify({'error': f'{field_name} must be a string (ISO 8601)'}), 400)
    try:
        return datetime.datetime.fromisoformat(dt_str.replace('Z', '+00:00')), None
    except (ValueError, TypeError):
        return None, (jsonify({'error': f'Invalid {field_name} format. Use ISO 8601'}), 400)

def _resolve_short_code(custom_code, code_length):
    if custom_code:
        if URL.query.filter_by(short_code=custom_code).first():
            return None, (jsonify({'error': 'Custom code already taken'}), 409)
        return custom_code, None

    short_code = generate_short_code(code_length)
    while URL.query.filter_by(short_code=short_code).first():
        short_code = generate_short_code(code_length)
    return short_code, None

def _validate_rotate_targets(targets):
    if targets is None:
        return None, None
    if not isinstance(targets, list) or not all(isinstance(u, str) for u in targets):
         return None, (jsonify({'error': 'rotate_targets must be a list of strings'}), 400)
    if len(targets) > 50:
         return None, (jsonify({'error': 'Maximum 50 rotate targets allowed'}), 400)

    targets = [u.strip() for u in targets]
    if not all(is_safe_url(u) for u in targets):
         return None, (jsonify({'error': 'One or more rotate target URLs are blocked or invalid.'}), 403)
    return targets, None

def _validate_custom_code(custom_code):
    if custom_code is None:
        return None, None
    if not isinstance(custom_code, str):
        return None, (jsonify({'error': 'custom_code must be a string'}), 400)
    custom_code = custom_code.strip().upper()
    if len(custom_code) < 3 or len(custom_code) > 20:
        return None, (jsonify({'error': 'custom_code must be between 3 and 20 characters'}), 400)
    if not re.match(r'^[A-Z0-9_-]+$', custom_code):
        return None, (jsonify({'error': 'custom_code must contain only alphanumeric characters, hyphens, or underscores'}), 400)
    return custom_code, None

def _get_code_length(data):
    try:
        code_length = int(data.get('code_length', current_app.config['SHORT_CODE_LENGTH']))
    except (ValueError, TypeError):
        return None, (jsonify({'error': 'code_length must be an integer'}), 400)

    if code_length < 3 or code_length > 20:
        return None, (jsonify({'error': 'code_length must be between 3 and 20'}), 400)
    return code_length, None

def _get_expiry_hours(data):
    try:
        expiry_hours = int(data.get('expiry_hours', current_app.config['EXPIRY_HOURS']))
    except (ValueError, TypeError):
        return None, (jsonify({'error': 'expiry_hours must be an integer'}), 400)

    if expiry_hours < 0 or expiry_hours > 876000: # 100 years
        return None, (jsonify({'error': 'expiry_hours must be between 0 and 876,000 (100 years)'}), 400)
    return expiry_hours, None

@api.route('/shorten', methods=['POST'])
@limiter.limit("60 per minute") # Higher limit for API
def shorten():
    # Authenticate User - Mandatory
    user = get_user_from_api_key()
    if not user:
        return jsonify({'error': 'Valid API Key required. Access denied.'}), 401

    data = request.get_json()
    if not isinstance(data, dict):
        return jsonify({'error': 'Request payload must be a JSON object'}), 400

    if 'long_url' not in data:
        return jsonify({'error': 'Missing long_url'}), 400

    if not isinstance(data.get('long_url'), str):
        return jsonify({'error': 'long_url must be a string'}), 400

    long_url = data['long_url'].strip()
    if not is_safe_url(long_url):
        return jsonify({'error': 'Destination URL is blocked'}), 403

    custom_code, error = _validate_custom_code(data.get('custom_code'))
    if error:
        return error

    code_length, error = _get_code_length(data)
    if error:
        return error
    
    expiry_hours, error = _get_expiry_hours(data)
    if error:
        return error

    preview_mode = data.get('preview_mode', True)
    stats_enabled = data.get('stats_enabled', True)
    
    start_at, error = _parse_iso_datetime(data.get('start_at'), 'start_at')
    if error:
        return error

    end_at, error = _parse_iso_datetime(data.get('end_at'), 'end_at')
    if error:
        return error

    if start_at and end_at and end_at <= start_at:
        return jsonify({'error': 'Invalid scheduling window: end_at must be after start_at'}), 400

    short_code, error = _resolve_short_code(custom_code, code_length)
    if error:
        return error

    expires_at = None
    if expiry_hours != 0:
        try:
            expires_at = datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=expiry_hours)
        except (OverflowError, OSError):
             return jsonify({'error': 'expiry_hours results in a date that is out of range'}), 400

    password = data.get('password')
    password_hash = generate_password_hash(password) if password else None

    rotate_targets, error = _validate_rotate_targets(data.get('rotate_targets'))
    if error:
        return error

    new_url = URL(
        user_id=user.id if user else None,
        short_code=short_code,
        long_url=long_url,
        rotate_targets=rotate_targets,
        password_hash=password_hash,
        preview_mode=preview_mode,
        stats_enabled=stats_enabled,
        expires_at=expires_at,
        start_at=start_at,
        end_at=end_at
    )
    db.session.add(new_url)
    db.session.commit()

    shortened_links_total.inc()

    short_url = f"https://{current_app.config['BASE_DOMAIN']}/{short_code}"
    return jsonify({
        'short_code': short_code,
        'short_url': short_url,
        'long_url': long_url,
        'rotate_targets': rotate_targets,
        'expires_at': expires_at.isoformat() if expires_at else None,
        'start_at': start_at.isoformat() if start_at else None,
        'end_at': end_at.isoformat() if end_at else None,
        'password_protected': bool(password),
        'preview_mode': preview_mode,
        'stats_enabled': stats_enabled
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
