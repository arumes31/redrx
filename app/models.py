import json
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
    created_at = db.Column(db.DateTime, default=lambda: datetime.now(timezone.utc))
    
    # Relationship to URLs
    urls = db.relationship('URL', backref='owner', lazy=True)

class URL(db.Model):
    __tablename__ = 'urls'
    id = db.Column(db.Integer, primary_key=True)
    user_id = db.Column(db.Integer, db.ForeignKey('users.id'), nullable=True) # Nullable for anonymous links
    short_code = db.Column(db.String(20), unique=True, nullable=False, index=True)
    long_url = db.Column(db.Text, nullable=False)
    _ab_urls = db.Column('ab_urls', db.Text, nullable=True) # Stored as JSON string
    password_hash = db.Column(db.String(255), nullable=True)
    clicks = db.Column(db.Integer, default=0)
    created_at = db.Column(db.DateTime, default=lambda: datetime.now(timezone.utc))
    expires_at = db.Column(db.DateTime, nullable=True)
    start_at = db.Column(db.DateTime, nullable=True)
    end_at = db.Column(db.DateTime, nullable=True)

    @property
    def ab_urls(self):
        if self._ab_urls:
            return json.loads(self._ab_urls)
        return []

    @ab_urls.setter
    def ab_urls(self, value):
        if value:
            self._ab_urls = json.dumps(value)
        else:
            self._ab_urls = None

    def is_active(self):
        now = datetime.now(timezone.utc).replace(tzinfo=None) # Compare naive to naive if db stores naive
        # SQLAlchemy defaults often store naive UTC. Let's ensure consistency.
        # If created_at is naive UTC from previous lambda, we should be careful.
        # However, typically SQLite stores strings.
        # Let's check if the fields are timezone aware or not.
        # By default SQLAlchemy DateTime is naive.
        # So we should use naive UTC for comparison if the DB is naive.
        
        # Let's stick to the pattern but make it naive for compatibility with default DateTime
        now = datetime.now(timezone.utc).replace(tzinfo=None)
        
        if self.start_at and now < self.start_at:
            return False
        if self.end_at and now > self.end_at:
            return False
        if self.expires_at and now > self.expires_at:
            return False
        return True
