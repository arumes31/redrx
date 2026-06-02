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

class APIError(Exception):
    def __init__(self, message, status_code=400):
        self.message = message
        self.status_code = status_code

@api.errorhandler(APIError)
def handle_api_error(error):
    return jsonify({'error': error.message}), error.status_code

def get_user_from_api_key():
    api_key = request.headers.get('X-API-KEY')
    if not api_key:
        return None
    return User.query.filter_by(api_key=api_key).first()

def _parse_iso_datetime(dt_str, field_name):
    if dt_str is None:
        return None
    if not isinstance(dt_str, str):
        raise APIError(f'{field_name} must be a string (ISO 8601)')
    try:
        return datetime.datetime.fromisoformat(dt_str.replace('Z', '+00:00'))
    except (ValueError, TypeError):
        raise APIError(f'Invalid {field_name} format. Use ISO 8601')

def _resolve_short_code(custom_code, code_length):
    if custom_code:
        if URL.query.filter_by(short_code=custom_code).first():
            raise APIError('Custom code already taken', 409)
        return custom_code

    short_code = generate_short_code(code_length)
    while URL.query.filter_by(short_code=short_code).first():
        short_code = generate_short_code(code_length)
    return short_code

def _validate_rotate_targets(targets):
    if targets is None:
        return None
    if not isinstance(targets, list) or not all(isinstance(u, str) for u in targets):
         raise APIError('rotate_targets must be a list of strings')
    if len(targets) > 50:
         raise APIError('Maximum 50 rotate targets allowed')

    targets = [u.strip() for u in targets]
    if not all(is_safe_url(u) for u in targets):
         raise APIError('One or more rotate target URLs are blocked or invalid.', 403)
    return targets

def _validate_custom_code(custom_code):
    if custom_code is None:
        return None
    if not isinstance(custom_code, str):
        raise APIError('custom_code must be a string')
    custom_code = custom_code.strip().upper()
    if len(custom_code) < 3 or len(custom_code) > 20:
        raise APIError('custom_code must be between 3 and 20 characters')
    if not re.match(r'^[A-Z0-9_-]+$', custom_code):
        raise APIError('custom_code must contain only alphanumeric characters, hyphens, or underscores')
    return custom_code

def _get_code_length(data):
    try:
        code_length = int(data.get('code_length', current_app.config['SHORT_CODE_LENGTH']))
    except (ValueError, TypeError):
        raise APIError('code_length must be an integer')

    if code_length < 3 or code_length > 20:
        raise APIError('code_length must be between 3 and 20')
    return code_length

def _get_expiry_hours(data):
    try:
        expiry_hours = int(data.get('expiry_hours', current_app.config['EXPIRY_HOURS']))
    except (ValueError, TypeError):
        raise APIError('expiry_hours must be an integer')

    if expiry_hours < 0 or expiry_hours > 876000: # 100 years
        raise APIError('expiry_hours must be between 0 and 876,000 (100 years)')
    return expiry_hours

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

    custom_code = _validate_custom_code(data.get('custom_code'))
    code_length = _get_code_length(data)
    expiry_hours = _get_expiry_hours(data)

    preview_mode = data.get('preview_mode', True)
    stats_enabled = data.get('stats_enabled', True)
    
    start_at = _parse_iso_datetime(data.get('start_at'), 'start_at')
    end_at = _parse_iso_datetime(data.get('end_at'), 'end_at')

    if start_at and end_at and end_at <= start_at:
        raise APIError('Invalid scheduling window: end_at must be after start_at')

    short_code = _resolve_short_code(custom_code, code_length)

    expires_at = None
    if expiry_hours != 0:
        try:
            expires_at = datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=expiry_hours)
        except (OverflowError, OSError):
             raise APIError('expiry_hours results in a date that is out of range')

    password = data.get('password')
    password_hash = generate_password_hash(password) if password else None

    rotate_targets = _validate_rotate_targets(data.get('rotate_targets'))

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
