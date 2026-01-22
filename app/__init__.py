from flask import Flask, request, redirect
from flask_login import LoginManager
from flask_limiter import Limiter
from flask_limiter.util import get_remote_address
from flask_wtf.csrf import CSRFProtect
from werkzeug.middleware.proxy_fix import ProxyFix
from prometheus_flask_exporter import PrometheusMetrics
from app.models import db, User
from config import Config
import os
import logging
import re
from urllib.parse import urlparse

login_manager = LoginManager()
login_manager.login_view = 'main.login'
login_manager.login_message_category = 'info'

csrf = CSRFProtect()

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
limit_redirect = os.environ.get('RATELIMIT_REDIRECT', "100 per minute")
limit_health = os.environ.get('RATELIMIT_HEALTH', "10 per minute")
storage_url = os.environ.get('RATELIMIT_STORAGE_URL', 'memory://')

limiter = Limiter(
    key_func=get_actual_ip,
    default_limits=[limit_default],
    storage_uri=storage_url
)

metrics = PrometheusMetrics.for_app_factory(path=None)

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
    metrics.init_app(app)
    csrf.init_app(app)

    @app.route('/metrics')
    @limiter.limit("10 per minute") # Strict limit for metrics to prevent abuse
    def custom_metrics():
        return metrics.export()

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
    from sqlalchemy import text, inspect

    with app.app_context():
        db.create_all()
        
        # Comprehensive schema verification
        try:
            inspector = inspect(db.engine)
            
            # Map of Table Name -> List of (Column Name, Column Type/SQL)
            required_schema = {
                'users': [
                    ('id', "INTEGER PRIMARY KEY"),
                    ('username', "VARCHAR(80)"),
                    ('email', "VARCHAR(120)"),
                    ('password_hash', "VARCHAR(255)"),
                    ('api_key', "VARCHAR(36)"),
                    ('created_at', "TIMESTAMP")
                ],
                'urls': [
                    ('id', "INTEGER PRIMARY KEY"),
                    ('user_id', "INTEGER"),
                    ('short_code', "VARCHAR(20)"),
                    ('long_url', "TEXT"),
                    ('rotate_targets', "TEXT"),
                    ('ios_target_url', "TEXT"),
                    ('android_target_url', "TEXT"),
                    ('password_hash', "VARCHAR(255)"),
                    ('preview_mode', "BOOLEAN DEFAULT TRUE"),
                    ('stats_enabled', "BOOLEAN DEFAULT TRUE"),
                    ('is_enabled', "BOOLEAN DEFAULT TRUE"),
                    ('clicks', "INTEGER DEFAULT 0"),
                    ('created_at', "TIMESTAMP"),
                    ('expires_at', "TIMESTAMP"),
                    ('start_at', "TIMESTAMP"),
                    ('end_at', "TIMESTAMP"),
                    ('last_accessed_at', "TIMESTAMP")
                ],
                'clicks': [
                    ('id', "INTEGER PRIMARY KEY"),
                    ('url_id', "INTEGER"),
                    ('timestamp', "TIMESTAMP"),
                    ('ip_address', "VARCHAR(45)"),
                    ('country', "VARCHAR(100)"),
                    ('browser', "VARCHAR(50)"),
                    ('platform', "VARCHAR(50)"),
                    ('referrer', "VARCHAR(255)")
                ]
            }

            with db.engine.connect() as conn:
                for table_name, columns in required_schema.items():
                    if not inspector.has_table(table_name):
                        app.logger.warning(f"Table '{table_name}' missing! create_all should have handled this.")
                        continue
                    
                    existing_columns = [c['name'] for c in inspector.get_columns(table_name)]
                    for col_name, col_def in columns:
                        if col_name not in existing_columns:
                            app.logger.info(f"Schema Sync: Adding missing column '{col_name}' to '{table_name}'...")
                            # Note: For simple types, we just use the name and basic type in ALTER
                            sql_type = col_def.replace("PRIMARY KEY", "") # Don't try to add PK via ALTER
                            conn.execute(text(f"ALTER TABLE {table_name} ADD COLUMN {col_name} {sql_type};"))
                            conn.commit()
                            app.logger.info(f"Schema Sync: Successfully added '{col_name}'.")
                            
        except Exception as e:
            app.logger.error(f"Schema verification failed: {e}")
            
        update_phishing_list()
        cleanup_phishing_urls()

    return app