from flask import Flask, request, redirect
from flask_login import LoginManager
from flask_limiter import Limiter
from flask_limiter.util import get_remote_address
from werkzeug.middleware.proxy_fix import ProxyFix
from app.models import db, User
from config import Config
import os
import logging
import re
from urllib.parse import urlparse

login_manager = LoginManager()
login_manager.login_view = 'main.login'
login_manager.login_message_category = 'info'

def get_actual_ip():
    """Custom key function for Limiter that respects Cloudflare headers."""
    # We check environment variable directly for the key function
    if os.environ.get('USE_CLOUDFLARE', 'false').lower() in ['true', '1', 't']:
        cf_ip = request.headers.get('CF-Connecting-IP')
        if cf_ip:
            return cf_ip
    return get_remote_address()

# Rate limiting configuration from env
limit_default = os.environ.get('RATELIMIT_DEFAULT', "200 per day;50 per hour")
limit_create = os.environ.get('RATELIMIT_CREATE', "10 per minute")
storage_url = os.environ.get('RATELIMIT_STORAGE_URL', 'memory://')

limiter = Limiter(
    key_func=get_actual_ip,
    default_limits=[limit_default],
    storage_uri=storage_url
)

class AnonymizeFilter(logging.Filter):
    def filter(self, record):
        if not os.environ.get('ANONYMIZE_LOGS', 'false').lower() in ['true', '1', 't']:
            return True
        
        # Mask IPv4
        ip_pattern = r'\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b'
        if isinstance(record.msg, str):
            record.msg = re.sub(ip_pattern, 'xxx.xxx.xxx.xxx', record.msg)
        
        if record.args:
            new_args = []
            for arg in record.args:
                if isinstance(arg, str):
                    new_args.append(re.sub(ip_pattern, 'xxx.xxx.xxx.xxx', arg))
                else:
                    new_args.append(arg)
            record.args = tuple(new_args)
        return True

def create_app(config_class=Config):
    app = Flask(__name__)
    app.config.from_object(config_class)

    # Apply anonymization filter if enabled
    if app.config.get('ANONYMIZE_LOGS'):
        for handler in app.logger.handlers:
            handler.addFilter(AnonymizeFilter())
        logging.getLogger('werkzeug').addFilter(AnonymizeFilter())

    # Reverse Proxy Support
    app.wsgi_app = ProxyFix(app.wsgi_app, x_for=1, x_proto=1, x_host=1, x_port=1, x_prefix=1)

    db.init_app(app)
    login_manager.init_app(app)
    limiter.init_app(app)

    @login_manager.user_loader
    def load_user(user_id):
        return User.query.get(int(user_id))

    @app.before_request
    def ensure_canonical_domain():
        # Skip if running in debug mode or localhost to avoid breaking dev env
        if app.debug or request.host.startswith('localhost') or request.host.startswith('127.0.0.1'):
            return

        base_domain = app.config.get('BASE_DOMAIN')
        if not base_domain:
            return

        # Clean base_domain in case it includes scheme
        if '://' in base_domain:
            base_domain = urlparse(base_domain).netloc

        if request.host != base_domain:
            # Reconstruct URL with canonical host
            # 301 Moved Permanently
            return redirect(request.url.replace(request.host, base_domain, 1), code=301)

    @app.after_request
    def set_security_headers(response):
        response.headers['Content-Security-Policy'] = "default-src 'self' 'unsafe-inline' 'unsafe-eval' https://fonts.googleapis.com https://fonts.gstatic.com data:;"
        response.headers['Strict-Transport-Security'] = 'max-age=31536000; includeSubDomains'
        response.headers['X-Frame-Options'] = 'SAMEORIGIN'
        response.headers['X-Content-Type-Options'] = 'nosniff'
        return response

    from app.routes import main as main_blueprint
    app.register_blueprint(main_blueprint)

    from app.api import api as api_blueprint
    app.register_blueprint(api_blueprint)

    from app.utils import update_phishing_list, cleanup_phishing_urls
    from sqlalchemy import text

    with app.app_context():
        db.create_all()
        
        # Auto-migration for device targeting columns and is_enabled
        try:
            with db.engine.connect() as conn:
                # Check/Add ios_target_url
                try:
                    conn.execute(text("ALTER TABLE urls ADD COLUMN ios_target_url TEXT;"))
                    conn.commit()
                    app.logger.info("Added ios_target_url column.")
                except Exception:
                    pass # Exists

                # Check/Add android_target_url
                try:
                    conn.execute(text("ALTER TABLE urls ADD COLUMN android_target_url TEXT;"))
                    conn.commit()
                    app.logger.info("Added android_target_url column.")
                except Exception:
                    pass # Exists
                    
                # Check/Add is_enabled
                try:
                    conn.execute(text("ALTER TABLE urls ADD COLUMN is_enabled BOOLEAN DEFAULT TRUE;"))
                    conn.commit()
                    app.logger.info("Added is_enabled column.")
                except Exception:
                    pass # Exists
                    
        except Exception as e:
            app.logger.error(f"Migration check failed: {e}")
            
        update_phishing_list()
        cleanup_phishing_urls()

    return app