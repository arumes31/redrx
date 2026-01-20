from flask_wtf import FlaskForm
from flask_wtf.file import FileField, FileAllowed
from wtforms import StringField, PasswordField, BooleanField, IntegerField, DateField, TimeField, TextAreaField, SubmitField
from wtforms.validators import DataRequired, URL, Optional, Length, Email

class ShortenURLForm(FlaskForm):
    long_url = StringField('Long URL', validators=[DataRequired(), URL(message="Invalid URL")])
    custom_code = StringField('Custom Code', validators=[Optional(), Length(min=3, max=20)])
    ab_urls = StringField('Alternate URLs', validators=[Optional()], description="Comma-separated URLs")
    password = PasswordField('Password', validators=[Optional()])
    
    # Expiry
    expiry_hours = IntegerField('Expiry (Hours)', default=24, validators=[Optional()])
    disable_expiry = BooleanField('Disable Expiry')
    
    # Scheduling
    start_date = DateField('Start Date', validators=[Optional()])
    start_time = TimeField('Start Time', validators=[Optional()])
    end_date = DateField('End Date', validators=[Optional()])
    end_time = TimeField('End Time', validators=[Optional()])
    
    # QR Customization
    qr_color = StringField('QR Color', default='#000000')
    qr_bg = StringField('QR Background', default='#ffffff')
    logo_file = FileField('QR Logo', validators=[FileAllowed(['png'], 'PNG Images only!')])
    
    submit = SubmitField('Shorten URL')

class BulkUploadForm(FlaskForm):
    csv_file = FileField('CSV File', validators=[DataRequired(), FileAllowed(['csv'], 'CSV Files only!')])
    submit = SubmitField('Process Bulk')

class LoginForm(FlaskForm):
    username = StringField('Username', validators=[DataRequired()])
    password = PasswordField('Password', validators=[DataRequired()])
    submit = SubmitField('Unlock')

class RegisterForm(FlaskForm):
    username = StringField('Username', validators=[DataRequired(), Length(min=4, max=20)])
    email = StringField('Email', validators=[DataRequired(), Email()])
    password = PasswordField('Password', validators=[DataRequired(), Length(min=6)])
    submit = SubmitField('Register')

class LinkPasswordForm(FlaskForm): # Renamed from original LoginForm to avoid confusion
    password = PasswordField('Password', validators=[DataRequired()])
    submit = SubmitField('Unlock')
