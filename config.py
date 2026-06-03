import os

# Compute module-level DEBUG and SECRET_KEY first
DEBUG = os.environ.get('FLASK_DEBUG', 'false').lower() in ['true', '1', 't']
SECRET_KEY = os.environ.get('SECRET_KEY')

if not SECRET_KEY:
    if DEBUG:
        # Fallback for development only
        SECRET_KEY = 'dev-secret-key-do-not-use-in-production'
    else:
        # Enforce security in production
        raise RuntimeError("SECRET_KEY must be set in production environments for security.")

class Config:
    basedir = os.path.abspath(os.path.dirname(__file__))

    # Environment mode
    DEBUG = DEBUG

    # Secret Key Security
    SECRET_KEY = SECRET_KEY

    SQLALCHEMY_DATABASE_URI = os.environ.get('DATABASE_URL') or \
        'sqlite:///' + os.path.join(basedir, 'db', 'shortener.db')
    SQLALCHEMY_TRACK_MODIFICATIONS = False
    MAX_CONTENT_LENGTH = 1 * 1024 * 1024 # 1MB Limit
    
    # App specific config
    BASE_DOMAIN = os.environ.get('BASE_DOMAIN', 'short.example.com')
    BLOCKED_DOMAINS = [b.strip().lower() for b in os.environ.get('BLOCKED_DOMAINS', '').split(',') if b.strip()]
    EXPIRY_HOURS = int(os.environ.get('EXPIRY_HOURS', 24))
    SHORT_CODE_LENGTH = int(os.environ.get('SHORT_CODE_LENGTH', 6))
    DEFAULT_QR_COLOR = os.environ.get('DEFAULT_QR_COLOR', 'black')
    DEFAULT_QR_BG = os.environ.get('DEFAULT_QR_BACKGROUND', 'white')
    GEOIP_DB_PATH = os.environ.get('GEOIP_DB_PATH', os.path.join(basedir, 'GeoLite2-Country.mmdb'))
    PHISHING_LIST_URLS = os.environ.get('PHISHING_LIST_URLS', 'https://raw.githubusercontent.com/mitchellkrogza/Phishing.Database/master/phishing-domains-ACTIVE.txt').split(',')
    BLOCKED_DOMAINS_PATH = os.environ.get('BLOCKED_DOMAINS_PATH', os.path.join(basedir, 'blocked_domains.txt'))
    PHISHING_CHECK_INTERVAL = int(os.environ.get('PHISHING_CHECK_INTERVAL', 24))
    ENABLE_PHISHING_CHECK = os.environ.get('ENABLE_PHISHING_CHECK', 'true').lower() in ['true', '1', 't']
    ENABLE_AUTO_REMOVE_PHISHING = os.environ.get('ENABLE_AUTO_REMOVE_PHISHING', 'false').lower() in ['true', '1', 't']
    PHISHING_REMOVE_INTERVAL = int(os.environ.get('PHISHING_REMOVE_INTERVAL', 24))
    DISABLE_ANONYMOUS_CREATE = os.environ.get('DISABLE_ANONYMOUS_CREATE', 'false').lower() in ['true', '1', 't']
    DISABLE_REGISTRATION = os.environ.get('DISABLE_REGISTRATION', 'false').lower() in ['true', '1', 't']
    USE_CLOUDFLARE = os.environ.get('USE_CLOUDFLARE', 'false').lower() in ['true', '1', 't']
    ANONYMIZE_LOGS = os.environ.get('ANONYMIZE_LOGS', 'false').lower() in ['true', '1', 't']
    ENABLE_SEO = os.environ.get('ENABLE_SEO', 'false').lower() in ['true', '1', 't']
    SEO_DOMAIN = os.environ.get('SEO_DOMAIN', 'redrx.eu')

    # Rate limiting
    RATELIMIT_DEFAULT = os.environ.get('RATELIMIT_DEFAULT', "200 per day;50 per hour")
    RATELIMIT_STORAGE_URI = os.environ.get('RATELIMIT_STORAGE_URL', 'memory://')
    RATELIMIT_LOGIN = os.environ.get('RATELIMIT_LOGIN', "10 per minute")
    RATELIMIT_REGISTER = os.environ.get('RATELIMIT_REGISTER', "5 per hour")
    RATELIMIT_AUTH = os.environ.get('RATELIMIT_AUTH', "10 per minute")
    RATELIMIT_API = os.environ.get('RATELIMIT_API', "60 per minute")
    RATELIMIT_CREATE = os.environ.get('RATELIMIT_CREATE', "10 per minute")
    RATELIMIT_REDIRECT = os.environ.get('RATELIMIT_REDIRECT', "100 per minute")
    RATELIMIT_HEALTH = os.environ.get('RATELIMIT_HEALTH', "10 per minute")
    RATELIMIT_METRICS = os.environ.get('RATELIMIT_METRICS', "10 per minute")
