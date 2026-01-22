from flask import Flask
from flask_login import LoginManager
from flask_limiter import Limiter
from flask_limiter.util import get_remote_address
from werkzeug.middleware.proxy_fix import ProxyFix
from app.models import db, User
from config import Config
import os
import logging
import re

login_manager = LoginManager()
login_manager.login_view = 'main.login'
login_manager.login_message_category = 'info'

# Rate limiting configuration from env
limit_default = os.environ.get('RATELIMIT_DEFAULT', "200 per day;50 per hour")
limit_create = os.environ.get('RATELIMIT_CREATE', "10 per minute")
storage_url = os.environ.get('RATELIMIT_STORAGE_URL', 'memory://')

limiter = Limiter(
    key_func=get_remote_address,
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

    from app.routes import main as main_blueprint
    app.register_blueprint(main_blueprint)

    from app.api import api as api_blueprint
    app.register_blueprint(api_blueprint)

    from app.utils import update_phishing_list, cleanup_phishing_urls

    with app.app_context():
        db.create_all()
        update_phishing_list()
        cleanup_phishing_urls()

    return app