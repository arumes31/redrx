import io
import csv
import json
import datetime
import uuid
from flask import Blueprint, render_template, request, redirect, url_for, flash, abort, send_file, current_app, session
from flask_login import login_user, logout_user, current_user, login_required
from flask_limiter import Limiter
from app import limiter # Import the instance
from werkzeug.security import generate_password_hash, check_password_hash
from PIL import Image

from app.models import db, URL, User, Click
from app.forms import ShortenURLForm, BulkUploadForm, LoginForm, RegisterForm, LinkPasswordForm, EditURLForm
from app.utils import generate_short_code, get_qr_data_url, generate_qr, select_rotate_target, get_geo_info, is_safe_url

main = Blueprint('main', __name__)

@main.route('/', methods=['GET', 'POST'])
@limiter.limit(lambda: current_app.config.get('RATELIMIT_CREATE', '10 per minute'), methods=['POST'])
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
        
        if not is_safe_url(long_url):
            flash("That destination URL is blocked for safety reasons.", 'danger')
            return render_template('index.html', form=form, bulk_form=bulk_form)

        custom_code = form.custom_code.data.strip().upper() if form.custom_code.data else None
        
        # Check Custom Code Availability
        if custom_code:
            if URL.query.filter_by(short_code=custom_code).first():
                flash(f"Code '{custom_code}' is already taken.", 'danger')
                return render_template('index.html', form=form, bulk_form=bulk_form)
            short_code = custom_code
        else:
            # Generate unique code
            length = form.code_length.data or current_app.config['SHORT_CODE_LENGTH']
            short_code = generate_short_code(length)
            while URL.query.filter_by(short_code=short_code).first():
                 short_code = generate_short_code(length)
        
        # Prepare Data
        rotate_list = [u.strip() for u in form.rotate_targets.data.split(',') if u.strip()] if form.rotate_targets.data else None
        
        password_hash = generate_password_hash(form.password.data) if form.password.data else None
        
        start_at = None
        if form.start_date.data and form.start_time.data:
            start_at = datetime.datetime.combine(form.start_date.data, form.start_time.data)
            
        end_at = None
        if form.end_date.data and form.end_time.data:
            end_at = datetime.datetime.combine(form.end_date.data, form.end_time.data)
            
        expires_at = None
        if not form.disable_expiry.data and form.expiry_hours.data != 0:
             expires_at = datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=form.expiry_hours.data)

        # Create Record
        new_url = URL(
            user_id=current_user.id if current_user.is_authenticated else None,
            short_code=short_code,
            long_url=long_url,
            rotate_targets=rotate_list,
            password_hash=password_hash,
            preview_mode=form.preview_mode.data,
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
    if url_entry.rotate_targets:
        alt = select_rotate_target(url_entry.rotate_targets)
        if alt:
            target_url = alt
            
    # Stats
    url_entry.clicks_count += 1
    
    # Record detailed click
    user_agent = request.user_agent
    new_click = Click(
        url_id=url_entry.id,
        ip_address=request.remote_addr,
        country=get_geo_info(request.remote_addr),
        browser=user_agent.browser,
        platform=user_agent.platform,
        referrer=request.referrer or "Direct"
    )
    db.session.add(new_click)
    db.session.commit()
    
    # If preview mode is enabled
    if url_entry.preview_mode:
        return render_template('preview.html', target_url=target_url, short_code=short_code)

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
@limiter.limit("5 per hour") # Prevent spam accounts
def register():
    if current_user.is_authenticated:
        return redirect(url_for('main.index'))
    form = RegisterForm()
    if form.validate_on_submit():
        hashed_password = generate_password_hash(form.password.data)
        api_key = str(uuid.uuid4())
        user = User(username=form.username.data, email=form.email.data, password_hash=hashed_password, api_key=api_key)
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

@main.route('/edit/<short_code>', methods=['GET', 'POST'])
@login_required
def edit_url(short_code):
    url_entry = URL.query.filter_by(short_code=short_code).first_or_404()
    
    if url_entry.user_id != current_user.id:
        abort(403)
        
    form = EditURLForm(obj=url_entry)
    
    # Manually handle 'active' logic as it's computed in model usually, 
    # but we might want to manually disable it. 
    # For now, let's allow changing long_url and maybe clearing expiry.
    # The current model doesn't have an explicit 'is_active' boolean column, it's computed.
    # So 'active' checkbox in form is tricky unless we add a column or manipulate dates.
    # Let's stick to editing Long URL for now to keep it simple and robust.
    
    if form.validate_on_submit():
        url_entry.long_url = form.long_url.data
        url_entry.preview_mode = form.preview_mode.data
        db.session.commit()
        flash('Link updated successfully.', 'success')
        return redirect(url_for('main.dashboard'))
        
    return render_template('edit_url.html', form=form, short_code=short_code)

@main.route('/<short_code>/stats')
def stats(short_code):
    url_entry = URL.query.filter_by(short_code=short_code).first_or_404()
    
    if not url_entry.is_active():
        # We allow viewing stats for expired links but warn
        pass
    
    short_url = f"https://{current_app.config['BASE_DOMAIN']}/{short_code}"
    
    # Process Analytics
    clicks = Click.query.filter_by(url_id=url_entry.id).order_by(Click.timestamp.asc()).all()
    
    # 1. Clicks over time (last 7 days by default)
    time_data = {}
    for click in clicks:
        day = click.timestamp.strftime('%Y-%m-%d')
        time_data[day] = time_data.get(day, 0) + 1
    
    # 2. Countries
    country_data = {}
    for click in clicks:
        country_data[click.country] = country_data.get(click.country, 0) + 1
    
    # 3. Browsers
    browser_data = {}
    for click in clicks:
        name = click.browser or "Unknown"
        browser_data[name] = browser_data.get(name, 0) + 1

    # 4. Platforms
    platform_data = {}
    for click in clicks:
        name = click.platform or "Unknown"
        platform_data[name] = platform_data.get(name, 0) + 1
        
    return render_template('stats.html', url=url_entry, short_url=short_url, 
                           active=url_entry.is_active(),
                           time_data=time_data,
                           country_data=country_data,
                           browser_data=browser_data,
                           platform_data=platform_data)

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
