import io
import csv
import json
import datetime
import uuid
from flask import Blueprint, render_template, request, redirect, url_for, flash, abort, send_file, current_app, session, jsonify, send_from_directory
from flask_login import login_user, logout_user, current_user, login_required
from flask_limiter import Limiter
from app import limiter # Import the instance
from werkzeug.security import generate_password_hash, check_password_hash
from PIL import Image
from user_agents import parse
from urllib.parse import urlparse

from app.models import db, URL, User, Click
from app.forms import ShortenURLForm, BulkUploadForm, LoginForm, RegisterForm, LinkPasswordForm, EditURLForm
from app.utils import generate_short_code, get_qr_data_url, generate_qr, select_rotate_target, get_geo_info, is_safe_url, get_client_ip

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
        if current_app.config.get('DISABLE_ANONYMOUS_CREATE') and not current_user.is_authenticated:
            flash("Please log in to shorten URLs.", 'warning')
            return redirect(url_for('main.login'))

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
        if form.expiry_hours.data and form.expiry_hours.data != 0:
             expires_at = datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=form.expiry_hours.data)

        # Create Record
        new_url = URL(
            user_id=current_user.id if current_user.is_authenticated else None,
            short_code=short_code,
            long_url=long_url,
            rotate_targets=rotate_list,
            password_hash=password_hash,
            preview_mode=form.preview_mode.data,
            stats_enabled=form.stats_enabled.data,
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

        short_url = f"https://{current_app.config['BASE_DOMAIN']}/{short_code}"

        qr_data = get_qr_data_url(
            short_url,
            color=form.qr_color.data,
            bg=form.qr_bg.data,
            logo_img=logo_img
        )
        
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
            
    # Update last accessed
    url_entry.last_accessed_at = datetime.datetime.now(datetime.timezone.utc)
    db.session.commit()

    # Stats
    if url_entry.stats_enabled:
        url_entry.clicks_count += 1
        
        # Record detailed click
        ua_string = request.headers.get('User-Agent')
        user_agent = parse(ua_string)
        client_ip = get_client_ip(request)
        
        new_click = Click(
            url_id=url_entry.id,
            ip_address=client_ip,
            country=get_geo_info(client_ip, request),
            browser=user_agent.browser.family,
            platform=user_agent.os.family,
            referrer=request.referrer or "Direct"
        )
        db.session.add(new_click)
        db.session.commit()
    
    # If preview mode is enabled
    if url_entry.preview_mode:
        return render_template('preview.html', target_url=target_url, short_code=short_code)

    return render_template('redirect.html', target_url=target_url)

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
    if current_app.config.get('DISABLE_REGISTRATION'):
        flash("Registration is currently disabled.", 'info')
        return redirect(url_for('main.index'))
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
        user = User.query.filter((User.username == form.username.data) | (User.email == form.username.data)).first()
        if user and check_password_hash(user.password_hash, form.password.data):
            login_user(user)
            next_page = request.args.get('next')
            return redirect(next_page) if next_page else redirect(url_for('main.index'))
        else:
            flash('Login Unsuccessful. Please check username/email and password', 'danger')
    return render_template('login_user.html', form=form)

@main.route('/logout')
def logout():
    logout_user()
    return redirect(url_for('main.index'))

@main.route('/dashboard')
@login_required
def dashboard():
    urls = URL.query.filter_by(user_id=current_user.id).order_by(URL.created_at.desc()).all()
    
    # Calculate stats
    total_links = len(urls)
    total_clicks = sum(u.clicks_count for u in urls)
    active_links = sum(1 for u in urls if u.is_active())
    top_performer = max(urls, key=lambda u: u.clicks_count) if urls else None
    
    stats = {
        'total_links': total_links,
        'total_clicks': total_clicks,
        'active_links': active_links,
        'top_performer': top_performer
    }
    
    return render_template('dashboard.html', urls=urls, stats=stats)

@main.route('/regenerate-api-key', methods=['POST'])
@login_required
def regenerate_api_key():
    current_user.api_key = str(uuid.uuid4())
    db.session.commit()
    flash('API Key regenerated successfully.', 'success')
    return redirect(url_for('main.dashboard'))

@main.route('/toggle-status/<short_code>', methods=['POST'])
@login_required
def toggle_status(short_code):
    url_entry = URL.query.filter_by(short_code=short_code).first_or_404()
    if url_entry.user_id != current_user.id:
        abort(403)
    url_entry.is_enabled = not url_entry.is_enabled
    db.session.commit()
    return jsonify({'status': 'success', 'is_enabled': url_entry.is_enabled})

@main.route('/export-links')
@login_required
def export_links():
    urls = URL.query.filter_by(user_id=current_user.id).all()
    
    output = io.StringIO()
    writer = csv.writer(output)
    writer.writerow(['Short Code', 'Long URL', 'Clicks', 'Created At', 'Last Accessed', 'Expires At'])
    
    for u in urls:
        writer.writerow([
            u.short_code, 
            u.long_url, 
            u.clicks_count, 
            u.created_at.isoformat(),
            u.last_accessed_at.isoformat() if u.last_accessed_at else 'Never',
            u.expires_at.isoformat() if u.expires_at else 'Never'
        ])
    
    output.seek(0)
    return send_file(
        io.BytesIO(output.getvalue().encode('utf-8')),
        mimetype='text/csv',
        as_attachment=True,
        download_name='my_links.csv'
    )

@main.route('/bulk-delete', methods=['POST'])
@login_required
def bulk_delete():
    ids = request.form.getlist('link_ids')
    if not ids:
        flash('No links selected.', 'warning')
        return redirect(url_for('main.dashboard'))
        
    URL.query.filter(URL.id.in_(ids), URL.user_id == current_user.id).delete(synchronize_session=False)
    db.session.commit()
    flash(f'Successfully deleted {len(ids)} links.', 'info')
    return redirect(url_for('main.dashboard'))

@main.route('/edit/<short_code>', methods=['GET', 'POST'])
@login_required
def edit_url(short_code):
    url_entry = URL.query.filter_by(short_code=short_code).first_or_404()
    
    if url_entry.user_id != current_user.id:
        abort(403)
        
    form = EditURLForm(obj=url_entry)
    
    if request.method == 'GET' and url_entry.expires_at:
        now = datetime.datetime.now(datetime.timezone.utc).replace(tzinfo=None)
        diff = url_entry.expires_at - now
        hours = int(diff.total_seconds() / 3600)
        form.expiry_hours.data = max(0, hours)
    
    if form.validate_on_submit():
        url_entry.long_url = form.long_url.data
        url_entry.preview_mode = form.preview_mode.data
        url_entry.stats_enabled = form.stats_enabled.data
        
        if form.expiry_hours.data is not None:
            if form.expiry_hours.data == 0:
                url_entry.expires_at = None
            else:
                url_entry.expires_at = datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=form.expiry_hours.data)
                
        db.session.commit()
        flash('Link updated successfully.', 'success')
        return redirect(url_for('main.dashboard'))
        
    return render_template('edit_url.html', form=form, short_code=short_code)

@main.route('/delete/<short_code>', methods=['POST'])
@login_required
def delete_url(short_code):
    url_entry = URL.query.filter_by(short_code=short_code).first_or_404()
    
    if url_entry.user_id != current_user.id:
        abort(403)
        
    db.session.delete(url_entry)
    db.session.commit()
    flash('Link deleted successfully.', 'info')
    return redirect(url_for('main.dashboard'))

@main.route('/<short_code>/stats')
def stats(short_code):
    url_entry = URL.query.filter_by(short_code=short_code).first_or_404()
    
    # Check ownership: If the URL has an owner, only that owner can see stats.
    # If it's anonymous (user_id is None), then anyone can see stats.
    current_user_id = current_user.id if current_user.is_authenticated else None
    if url_entry.user_id != current_user_id:
        abort(403)
        
    range_type = request.args.get('range', '30d')
    now = datetime.datetime.now(datetime.timezone.utc)
    
    # Process Analytics
    clicks = Click.query.filter_by(url_id=url_entry.id).order_by(Click.timestamp.asc()).all()
    
    # 1. Clicks over time (Hybrid logic based on range_type)
    time_data = {}
    if range_type == '24h':
        cutoff = now - datetime.timedelta(hours=24)
        for i in range(24, -1, -1):
            time_data[(now - datetime.timedelta(hours=i)).strftime('%H:00')] = 0
    elif range_type == '7d':
        cutoff = now - datetime.timedelta(days=7)
        for i in range(7, -1, -1):
            time_data[(now - datetime.timedelta(days=i)).strftime('%Y-%m-%d')] = 0
    else: # 30d
        cutoff = now - datetime.timedelta(days=30)
        for i in range(30, -1, -1):
            time_data[(now - datetime.timedelta(days=i)).strftime('%Y-%m-%d')] = 0

    # Filter clicks by range and populate time_data
    filtered_clicks = []
    referrer_data = {}
    country_data = {}
    browser_data = {}
    platform_data = {}

    for click in clicks:
        click_time = click.timestamp.replace(tzinfo=datetime.timezone.utc) if click.timestamp.tzinfo is None else click.timestamp
        
        # Global stats (all time)
        country_data[click.country] = country_data.get(click.country, 0) + 1
        browser_data[click.browser or "Unknown"] = browser_data.get(click.browser or "Unknown", 0) + 1
        platform_data[click.platform or "Unknown"] = platform_data.get(click.platform or "Unknown", 0) + 1
        
        # Referrer (Step 1)
        ref = click.referrer or "Direct"
        if "://" in ref:
            ref = urlparse(ref).netloc or ref
        referrer_data[ref] = referrer_data.get(ref, 0) + 1

        # Range-specific trend data (Step 3)
        if click_time >= cutoff:
            filtered_clicks.append(click)
            if range_type == '24h':
                key = click_time.strftime('%H:00')
            else:
                key = click_time.strftime('%Y-%m-%d')
            if key in time_data:
                time_data[key] += 1

    # Step 6: Momentum (Average clicks per day in range)
    days_in_range = 1 if range_type == '24h' else (7 if range_type == '7d' else 30)
    avg_daily = round(len(filtered_clicks) / days_in_range, 1)

    # Step 4: IP Anonymization and Relative Time for recent activity (last 10)
    recent_clicks = Click.query.filter_by(url_id=url_entry.id).order_by(Click.timestamp.desc()).limit(10).all()
    now_utc = datetime.datetime.now(datetime.timezone.utc)
    for rc in recent_clicks:
        # Relative time
        diff = now_utc - rc.timestamp.replace(tzinfo=datetime.timezone.utc)
        if diff.days > 0:
            rc.relative_time = f"{diff.days}d ago"
        elif diff.seconds >= 3600:
            rc.relative_time = f"{diff.seconds // 3600}h ago"
        elif diff.seconds >= 60:
            rc.relative_time = f"{diff.seconds // 60}m ago"
        else:
            rc.relative_time = "Just now"

        if rc.ip_address:
            parts = rc.ip_address.split('.')
            if len(parts) == 4:
                rc.ip_anonymized = f"{parts[0]}.{parts[1]}.xxx.xxx"
            else: # IPv6
                v6_parts = rc.ip_address.split(':')
                if len(v6_parts) >= 2:
                    rc.ip_anonymized = f"{v6_parts[0]}:{v6_parts[1]}:xxxx:xxxx"
                else:
                    rc.ip_anonymized = rc.ip_address[:rc.ip_address.find(':')+1] + "xxxx" if ':' in rc.ip_address else "xxxx"
        else:
            rc.ip_anonymized = "Unknown"

    short_url = f"https://{current_app.config['BASE_DOMAIN']}/{short_code}"
    return render_template('stats.html', url=url_entry, short_url=short_url, 
                           active=url_entry.is_active(),
                           time_data=time_data,
                           country_data=country_data,
                           browser_data=browser_data,
                           platform_data=platform_data,
                           referrer_data=referrer_data,
                           avg_daily=avg_daily,
                           range_type=range_type,
                           recent_clicks=recent_clicks)

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

@main.route('/robots.txt')
def robots():
    if not current_app.config.get('ENABLE_SEO'):
        abort(404)
    return send_from_directory(current_app.static_folder, 'robots.txt')

@main.route('/sitemap.xml')
def sitemap():
    if not current_app.config.get('ENABLE_SEO'):
        abort(404)
    return send_from_directory(current_app.static_folder, 'sitemap.xml')
