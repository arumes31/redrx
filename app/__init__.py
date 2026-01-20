from flask import Flask
from app.models import db
from config import Config

def create_app(config_class=Config):
    app = Flask(__name__)
    app.config.from_object(config_class)

    db.init_app(app)

    from app.routes import main as main_blueprint
    app.register_blueprint(main_blueprint)

    from app.api import api as api_blueprint
    app.register_blueprint(api_blueprint)

    with app.app_context():
        db.create_all()

    return app
