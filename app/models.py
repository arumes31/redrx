import json
from datetime import datetime
from flask_sqlalchemy import SQLAlchemy

db = SQLAlchemy()

class URL(db.Model):
    __tablename__ = 'urls'

    id = db.Column(db.Integer, primary_key=True)
    short_code = db.Column(db.String(20), unique=True, nullable=False, index=True)
    long_url = db.Column(db.Text, nullable=False)
    _ab_urls = db.Column('ab_urls', db.Text, nullable=True) # Stored as JSON string
    password_hash = db.Column(db.String(255), nullable=True)
    clicks = db.Column(db.Integer, default=0)
    created_at = db.Column(db.DateTime, default=datetime.utcnow)
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
        now = datetime.utcnow()
        if self.start_at and now < self.start_at:
            return False
        if self.end_at and now > self.end_at:
            return False
        if self.expires_at and now > self.expires_at:
            return False
        return True
