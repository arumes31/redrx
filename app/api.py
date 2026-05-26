from flask import Blueprint, request, jsonify, current_app
from werkzeug.security import generate_password_hash
from app.models import db, URL, User
from app.utils import generate_short_code, generate_qr, is_safe_url
from app import limiter, csrf
from app.routes import shortened_links_total # Import the custom counter
import datetime
import base64

api = Blueprint('api', __name__, url_prefix='/api/v1')
csrf.exempt(api)

def get_user_from_api_key():
    api_key = request.headers.get('X-API-KEY')
    if not api_key:
        return None
    return User.query.filter_by(api_key=api_key).first()

def _get_or_generate_short_code(custom_code, code_length):
    if custom_code:
        custom_code = custom_code.strip().upper()
        if URL.query.filter_by(short_code=custom_code).first():
            return None, 'Custom code already taken', 409
        return custom_code, None, None

    short_code = generate_short_code(code_length)
    while URL.query.filter_by(short_code=short_code).first():
        short_code = generate_short_code(code_length)
    return short_code, None, None

def _parse_timing_and_expiry(expiry_hours, start_at_str, end_at_str):
    expires_at = None
    try:
        if int(expiry_hours) != 0:
            expires_at = datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=int(expiry_hours))
    except (ValueError, TypeError):
         return None, None, None, 'Invalid expiry_hours', 400

    start_at = None
    if start_at_str:
        try:
            start_at = datetime.datetime.fromisoformat(start_at_str.replace('Z', '+00:00'))
        except ValueError:
            return None, None, None, 'Invalid start_at format. Use ISO 8601', 400

    end_at = None
    if end_at_str:
        try:
            end_at = datetime.datetime.fromisoformat(end_at_str.replace('Z', '+00:00'))
        except ValueError:
            return None, None, None, 'Invalid end_at format. Use ISO 8601', 400

    return expires_at, start_at, end_at, None, None

def _validate_rotate_targets(rotate_targets):
    if rotate_targets is None:
        return None, None, None

    if not isinstance(rotate_targets, list) or not all(isinstance(u, str) for u in rotate_targets):
         return None, 'rotate_targets must be a list of strings', 400

    if len(rotate_targets) > 50:
         return None, 'Maximum 50 rotate targets allowed', 400

    rotate_targets = [u.strip() for u in rotate_targets]
    if not all(is_safe_url(u) for u in rotate_targets):
         return None, 'One or more rotate target URLs are blocked or invalid.', 403

    return rotate_targets, None, None

@api.route('/shorten', methods=['POST'])
@limiter.limit("60 per minute") # Higher limit for API
def shorten():
    # Authenticate User - Mandatory
    user = get_user_from_api_key()
    
    if not user:
        return jsonify({'error': 'Valid API Key required. Access denied.'}), 401

    data = request.get_json()
    if not data or 'long_url' not in data:
        return jsonify({'error': 'Missing long_url'}), 400

    long_url = data['long_url'].strip()
    if not is_safe_url(long_url):
        return jsonify({'error': 'Destination URL is blocked'}), 403

    # Short code logic
    custom_code = data.get('custom_code')
    try:
        code_length = int(data.get('code_length', current_app.config['SHORT_CODE_LENGTH']))
    except (ValueError, TypeError):
        return jsonify({'error': 'Invalid code_length'}), 400

    short_code, err_msg, status = _get_or_generate_short_code(custom_code, code_length)
    if err_msg:
        return jsonify({'error': err_msg}), status

    # Timing and Expiry logic
    expiry_hours = data.get('expiry_hours', current_app.config['EXPIRY_HOURS'])
    expires_at, start_at, end_at, err_msg, status = _parse_timing_and_expiry(
        expiry_hours, data.get('start_at'), data.get('end_at')
    )
    if err_msg:
        return jsonify({'error': err_msg}), status

    # Rotate targets logic
    rotate_targets, err_msg, status = _validate_rotate_targets(data.get('rotate_targets'))
    if err_msg:
        return jsonify({'error': err_msg}), status

    # Password hashing
    password = data.get('password')
    password_hash = generate_password_hash(password) if password else None

    new_url = URL(
        user_id=user.id,
        short_code=short_code,
        long_url=long_url,
        rotate_targets=rotate_targets,
        password_hash=password_hash,
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
    # Authenticate User - Mandatory
    user = get_user_from_api_key()
    if not user:
        return jsonify({'error': 'Valid API Key required. Access denied.'}), 401

    url_entry = URL.query.filter_by(short_code=short_code.upper()).first()
    if not url_entry:
        return jsonify({'error': 'URL not found'}), 404

    return jsonify({
        'short_code': url_entry.short_code,
        'long_url': url_entry.long_url,
        'clicks': url_entry.clicks,
        'created_at': url_entry.created_at.isoformat(),
        'expires_at': url_entry.expires_at.isoformat() if url_entry.expires_at else None,
        'active': url_entry.is_active()
    })
