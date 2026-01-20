import io
import csv
import json
import datetime
from flask import Blueprint, render_template, request, redirect, url_for, flash, abort, send_file, current_app, session
from flask_login import login_user, logout_user, current_user, login_required
from werkzeug.security import generate_password_hash, check_password_hash
from PIL import Image

from app.models import db, URL, User
from app.forms import ShortenURLForm, BulkUploadForm, LoginForm, RegisterForm, LinkPasswordForm
from app.utils import generate_short_code, get_qr_data_url, generate_qr, select_ab_url

main = Blueprint('main', __name__)

@main.route('/', methods=['GET', 'POST'])
def index():
    form = ShortenURLForm()
    bulk_form = BulkUploadForm()
    
    short_url = None
    qr_data = None
    stats_url = None
    
    # Handle Tab Switching Logic via Session or just context if needed, 
    # but strictly forms handle their own validation.
    
    if form.validate_on_submit():
        long_url = form.long_url.data
        custom_code = form.custom_code.data.strip().upper() if form.custom_code.data else None
        
        # Check Custom Code Availability
        if custom_code:
            if URL.query.filter_by(short_code=custom_code).first():
                flash(f"Code '{custom_code}' is already taken.", 'danger')
                return render_template('index.html', form=form, bulk_form=bulk_form)
            short_code = custom_code
        else:
            # Generate unique code
            length = current_app.config['SHORT_CODE_LENGTH']
            short_code = generate_short_code(length)
            while URL.query.filter_by(short_code=short_code).first():
                 short_code = generate_short_code(length)
        
        # Prepare Data
        ab_list = [u.strip() for u in form.ab_urls.data.split(',') if u.strip()] if form.ab_urls.data else None
        
        password_hash = generate_password_hash(form.password.data) if form.password.data else None
        
        start_at = None
        if form.start_date.data and form.start_time.data:
            start_at = datetime.datetime.combine(form.start_date.data, form.start_time.data)
            
        end_at = None
        if form.end_date.data and form.end_time.data:
            end_at = datetime.datetime.combine(form.end_date.data, form.end_time.data)
            
        expires_at = None
        if not form.disable_expiry.data:
             expires_at = datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=form.expiry_hours.data)

        # Create Record
        new_url = URL(
            user_id=current_user.id if current_user.is_authenticated else None,
            short_code=short_code,
            long_url=long_url,
            ab_urls=ab_list,
            password_hash=password_hash,
            start_at=start_at,
            end_at=end_at,
            expires_at=expires_at
        )
        db.session.add(new_url)
        db.session.commit()
        
        # Generate QR
        logo_img = None
        if form.logo_file.data:
             try:
                logo_img = Image.open(form.logo_file.data)
             except Exception:
                 flash("Invalid Logo Image", 'warning')

        qr_data = get_qr_data_url(
            f"https://{current_app.config['BASE_DOMAIN']}/{short_code}",
            color=form.qr_color.data,
            bg=form.qr_bg.data,
            logo_img=logo_img
        )
        
        short_url = f"https://{current_app.config['BASE_DOMAIN']}/{short_code}"
        stats_url = url_for('main.stats', short_code=short_code, _external=True)
        
        flash("URL Shortened Successfully!", 'success')
        
    return render_template('index.html', form=form, bulk_form=bulk_form, 
                           short_url=short_url, qr_data=qr_data, stats_url=stats_url)

@main.route('/bulk', methods=['POST'])
def bulk_upload():
    form = BulkUploadForm() # We just use this for validation check mostly
    # Logic is slightly different as it comes from a different form submission in the UI usually
    # But we can handle it here or in index. Let's handle it here for cleanliness if the UI posts here.
    # However, keeping it single-page is better.
    # Let's assume the index template posts to / for single and /bulk for bulk to keep it clean.
    
    if 'csv_file' not in request.files:
        flash('No file part', 'danger')
        return redirect(url_for('main.index'))
        
    file = request.files['csv_file']
    if file.filename == '':
        flash('No selected file', 'danger')
        return redirect(url_for('main.index'))
        
    if file:
        try:
            stream = io.StringIO(file.stream.read().decode("UTF8"), newline=None)
            csv_reader = csv.DictReader(stream)
            results = []
            errors = []
            
            for row in csv_reader:
                long_url = row.get('long_url', '').strip()
                if not long_url:
                    errors.append("Row missing 'long_url'")
                    continue
                
                custom_code = row.get('custom_code', '').strip().upper()
                if custom_code and URL.query.filter_by(short_code=custom_code).first():
                    # Generate random if taken or skip? Let's generate random to be safe
                    custom_code = generate_short_code(current_app.config['SHORT_CODE_LENGTH'])
                elif not custom_code:
                     custom_code = generate_short_code(current_app.config['SHORT_CODE_LENGTH'])
                
                # Default expiry
                expires_at = datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=current_app.config['EXPIRY_HOURS'])
                
                new_url = URL(
                    short_code=custom_code,
                    long_url=long_url,
                    expires_at=expires_at
                )
                db.session.add(new_url)
                results.append(f"https://{current_app.config['BASE_DOMAIN']}/{custom_code}")
            
            db.session.commit()
            flash(f"Processed {len(results)} URLs.", 'success')
            return render_template('bulk_results.html', results=results, errors=errors)
            
        except Exception as e:
            flash(f"Error processing CSV: {e}", 'danger')
            return redirect(url_for('main.index'))

    return redirect(url_for('main.index'))

@main.route('/<short_code>')
def redirect_to_url(short_code):
    url_entry = URL.query.filter_by(short_code=short_code).first()
    
    if not url_entry:
        abort(404)
        
    if not url_entry.is_active():
        abort(410)
        
    if url_entry.password_hash:
        # Check session
        auth_key = f"auth_{short_code}"
        if not session.get(auth_key):
             return redirect(url_for('main.link_password_auth', short_code=short_code))
             
    # Select destination
    target_url = url_entry.long_url
    if url_entry.ab_urls:
        alt = select_ab_url(url_entry.ab_urls)
        if alt:
            target_url = alt
            
    # Stats
    url_entry.clicks += 1
    db.session.commit()
    
    return redirect(target_url, code=302)

@main.route('/link-auth/<short_code>', methods=['GET', 'POST'])
def link_password_auth(short_code):
    url_entry = URL.query.filter_by(short_code=short_code).first_or_404()
    form = LinkPasswordForm()
    
    if form.validate_on_submit():
        if check_password_hash(url_entry.password_hash, form.password.data):
            session[f"auth_{short_code}"] = True
            return redirect(url_for('main.redirect_to_url', short_code=short_code))
        else:
            flash("Invalid Password", 'danger')
            
    return render_template('login.html', form=form, short_code=short_code)

@main.route('/register', methods=['GET', 'POST'])
def register():
    if current_user.is_authenticated:
        return redirect(url_for('main.index'))
    form = RegisterForm()
    if form.validate_on_submit():
        hashed_password = generate_password_hash(form.password.data)
        user = User(username=form.username.data, email=form.email.data, password_hash=hashed_password)
        db.session.add(user)
        db.session.commit()
        flash('Your account has been created! You can now log in.', 'success')
        return redirect(url_for('main.login'))
    return render_template('register.html', form=form)

@main.route('/login', methods=['GET', 'POST'])
def login():
    if current_user.is_authenticated:
        return redirect(url_for('main.index'))
    form = LoginForm()
    if form.validate_on_submit():
        user = User.query.filter_by(username=form.username.data).first()
        if user and check_password_hash(user.password_hash, form.password.data):
            login_user(user)
            next_page = request.args.get('next')
            return redirect(next_page) if next_page else redirect(url_for('main.index'))
        else:
            flash('Login Unsuccessful. Please check username and password', 'danger')
    return render_template('login_user.html', form=form)

@main.route('/logout')
def logout():
    logout_user()
    return redirect(url_for('main.index'))

@main.route('/dashboard')
@login_required
def dashboard():
    urls = URL.query.filter_by(user_id=current_user.id).order_by(URL.created_at.desc()).all()
    return render_template('dashboard.html', urls=urls)

@main.route('/<short_code>/stats')
def stats(short_code):
    url_entry = URL.query.filter_by(short_code=short_code).first_or_404()
    
    if not url_entry.is_active():
        abort(410) # Or show stats for expired links? Let's show stats usually.
        # But original logic aborted 410. Let's allow stats for expired links but show status.
    
    short_url = f"https://{current_app.config['BASE_DOMAIN']}/{short_code}"
    
    return render_template('stats.html', url=url_entry, short_url=short_url, 
                           active=url_entry.is_active())

@main.route('/<short_code>/qr')
def qr_download(short_code):
    url_entry = URL.query.filter_by(short_code=short_code).first_or_404()
    short_url = f"https://{current_app.config['BASE_DOMAIN']}/{short_code}"
    
    # Generate simple QR for download (black/white usually best for raw download, or use defaults)
    img_buffer = generate_qr(short_url)
    return send_file(img_buffer, mimetype='image/png', as_attachment=False, download_name=f'{short_code}.png')

@main.errorhandler(404)
def not_found(e):
    return render_template('404.html'), 404

@main.errorhandler(410)
def gone(e):
    return render_template('410.html'), 410
