import os

class Config:
    SECRET_KEY = os.environ.get('SECRET_KEY') or os.urandom(24)
    basedir = os.path.abspath(os.path.dirname(__file__))
    SQLALCHEMY_DATABASE_URI = os.environ.get('DATABASE_URL') or \
        'sqlite:///' + os.path.join(basedir, 'db', 'shortener.db')
    SQLALCHEMY_TRACK_MODIFICATIONS = False
    
    # App specific config
    BASE_DOMAIN = os.environ.get('BASE_DOMAIN', 'short.example.com')
    EXPIRY_HOURS = int(os.environ.get('EXPIRY_HOURS', 24))
    SHORT_CODE_LENGTH = int(os.environ.get('SHORT_CODE_LENGTH', 6))
    DEFAULT_QR_COLOR = os.environ.get('DEFAULT_QR_COLOR', 'black')
    DEFAULT_QR_BG = os.environ.get('DEFAULT_QR_BACKGROUND', 'white')
    DEBUG = os.environ.get('FLASK_DEBUG', 'false').lower() in ['true', '1', 't']

