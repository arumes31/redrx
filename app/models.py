import json
import uuid
from datetime import datetime, timezone
from flask_sqlalchemy import SQLAlchemy
from flask_login import UserMixin

db = SQLAlchemy()

class User(db.Model, UserMixin):
    __tablename__ = 'users'
    id = db.Column(db.Integer, primary_key=True)
    username = db.Column(db.String(80), unique=True, nullable=False, index=True)
    email = db.Column(db.String(120), unique=True, nullable=False, index=True)
    password_hash = db.Column(db.String(255), nullable=False)
    api_key = db.Column(db.String(36), unique=True, nullable=True, index=True)
    created_at = db.Column(db.DateTime, default=lambda: datetime.now(timezone.utc))
    
    # Relationship to URLs
    urls = db.relationship('URL', backref='owner', lazy=True)

class URL(db.Model):
    __tablename__ = 'urls'
    id = db.Column(db.Integer, primary_key=True)
    user_id = db.Column(db.Integer, db.ForeignKey('users.id'), nullable=True) # Nullable for anonymous links
    short_code = db.Column(db.String(20), unique=True, nullable=False, index=True)
    long_url = db.Column(db.Text, nullable=False)
    _rotate_targets = db.Column('rotate_targets', db.Text, nullable=True) # Stored as JSON string
    password_hash = db.Column(db.String(255), nullable=True)
    preview_mode = db.Column(db.Boolean, default=True)
    stats_enabled = db.Column(db.Boolean, default=True)
    is_enabled = db.Column(db.Boolean, default=True)
    clicks_count = db.Column('clicks', db.Integer, default=0)
    created_at = db.Column(db.DateTime, default=lambda: datetime.now(timezone.utc))
    expires_at = db.Column(db.DateTime, nullable=True)
    start_at = db.Column(db.DateTime, nullable=True)
    end_at = db.Column(db.DateTime, nullable=True)
    last_accessed_at = db.Column(db.DateTime, nullable=True)

    # Relationship to detailed clicks
    clicks = db.relationship('Click', backref='url', lazy=True, cascade="all, delete-orphan")

    @property
    def rotate_targets(self):
        if self._rotate_targets:
            return json.loads(self._rotate_targets)
        return []

    @rotate_targets.setter
    def rotate_targets(self, value):
        if value:
            self._rotate_targets = json.dumps(value)
        else:
            self._rotate_targets = None

    def is_active(self):
        if not self.is_enabled:
            return False
            
        now = datetime.now(timezone.utc).replace(tzinfo=None)
        
        if self.start_at and now < self.start_at:
            return False
        if self.end_at and now > self.end_at:
            return False
        if self.expires_at and now > self.expires_at:
            return False
        return True

class Click(db.Model):
    __tablename__ = 'clicks'
    id = db.Column(db.Integer, primary_key=True)
    url_id = db.Column(db.Integer, db.ForeignKey('urls.id'), nullable=False)
    timestamp = db.Column(db.DateTime, default=lambda: datetime.now(timezone.utc))
    ip_address = db.Column(db.String(45))
    country = db.Column(db.String(100), default="Unknown")
    browser = db.Column(db.String(50))
    platform = db.Column(db.String(50))
    referrer = db.Column(db.String(255), default="Direct")
