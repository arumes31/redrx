import os

class Config:
    SECRET_KEY = os.environ.get('SECRET_KEY') or os.urandom(24)
    basedir = os.path.abspath(os.path.dirname(__file__))
    SQLALCHEMY_DATABASE_URI = os.environ.get('DATABASE_URL') or \
        'sqlite:///' + os.path.join(basedir, 'db', 'shortener.db')
    SQLALCHEMY_TRACK_MODIFICATIONS = False
    MAX_CONTENT_LENGTH = 1 * 1024 * 1024 # 1MB Limit
    
    # App specific config
    BASE_DOMAIN = os.environ.get('BASE_DOMAIN', 'short.example.com')
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
    DEBUG = os.environ.get('FLASK_DEBUG', 'false').lower() in ['true', '1', 't']

