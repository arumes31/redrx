import io
import csv
import datetime
import uuid
from flask import Blueprint, render_template, request, redirect, url_for, flash, abort, send_file, current_app, session, jsonify
from flask_login import login_user, logout_user, current_user, login_required
from app import limiter
from werkzeug.security import generate_password_hash, check_password_hash
from user_agents import parse
from urllib.parse import urlparse
from sqlalchemy import text
from prometheus_client import Counter

# Custom Metrics
shortened_links_total = Counter('redrx_shortened_links_total', 'Total number of shortened links created')
redirections_total = Counter('redrx_redirections_total', 'Total number of link redirections')
ratelimit_hits_total = Counter('redrx_ratelimit_hits_total', 'Total number of requests hitting the rate limit')

from app.models import db, URL, User, Click
from app.forms import ShortenURLForm, LoginForm, RegisterForm, LinkPasswordForm, EditURLForm
from app.utils import (
    generate_short_code, generate_qr, is_safe_url,
    get_geo_info, get_client_ip
)

main = Blueprint('main', __name__)

@main.route('/health')
@limiter.limit(lambda: current_app.config.get('RATELIMIT_HEALTH', '10 per minute'))
def health_check():
    try:
        # Check DB
        db.session.execute(text('SELECT 1'))
        return jsonify({
            'status': 'healthy',
            'timestamp': datetime.datetime.now(datetime.timezone.utc).isoformat(),
            'database': 'connected'
        }), 200
    except Exception as e:
        return jsonify({
            'status': 'unhealthy',
            'error': str(e),
            'timestamp': datetime.datetime.now(datetime.timezone.utc).isoformat()
        }), 500

@main.route('/', methods=['GET', 'POST'])
@limiter.limit(lambda: current_app.config.get('RATELIMIT_CREATE', '10 per minute'), methods=['POST'])
@limiter.limit(lambda: current_app.config.get('RATELIMIT_DEFAULT', '200 per day'), methods=['GET'])
def index():
    form = ShortenURLForm()
    if form.validate_on_submit():
        long_url = form.long_url.data
        custom_code = form.custom_code.data
        password = form.password.data
        
        # Security: Check for malicious URLs
        if not is_safe_url(long_url):
            flash("That destination URL is blocked or invalid.", 'danger')
            return render_template('index.html', form=form)

        if custom_code:
            if URL.query.filter_by(short_code=custom_code).first():
                flash('Custom code already exists. Please choose another one.', 'danger')
                return render_template('index.html', form=form)
            short_code = custom_code
        else:
            short_code = generate_short_code()
            while URL.query.filter_by(short_code=short_code).first():
                short_code = generate_short_code()
        
        expires_at = None
        if form.expiry_hours.data:
            expires_at = datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=form.expiry_hours.data)

        new_url = URL(
            long_url=long_url,
            short_code=short_code,
            user_id=current_user.id if current_user.is_authenticated else None,
            expires_at=expires_at,
            preview_mode=form.preview_mode.data
        )

        if password:
            new_url.password_hash = generate_password_hash(password)

        db.session.add(new_url)
        db.session.commit()
        
        # Increment Prometheus Counter
        shortened_links_total.inc()
        
        short_url = f"https://{current_app.config['BASE_DOMAIN']}/{short_code}"
        return render_template('index.html', form=form, short_url=short_url)
    
    return render_template('index.html', form=form)

@main.route('/login', methods=['GET', 'POST'])
@limiter.limit(lambda: current_app.config.get('RATELIMIT_LOGIN', '10 per minute'))
def login():
    if current_user.is_authenticated:
        return redirect(url_for('main.dashboard'))
    form = LoginForm()
    if form.validate_on_submit():
        user = User.query.filter_by(username=form.username.data).first()
        if user and check_password_hash(user.password_hash, form.password.data):
            login_user(user, remember=form.remember.data)
            return redirect(url_for('main.dashboard'))
        flash('Login Unsuccessful. Please check username and password', 'danger')
    return render_template('login.html', form=form)

@main.route('/register', methods=['GET', 'POST'])
@limiter.limit(lambda: current_app.config.get('RATELIMIT_REGISTER', '5 per hour'))
def register():
    if current_user.is_authenticated:
        return redirect(url_for('main.dashboard'))
    form = RegisterForm()
    if form.validate_on_submit():
        hashed_password = generate_password_hash(form.password.data)
        user = User(username=form.username.data, email=form.email.data, password_hash=hashed_password)
        api_key = str(uuid.uuid4())
        user.api_key = api_key
        db.session.add(user)
        db.session.commit()
        flash('Your account has been created! You are now able to log in', 'success')
        return redirect(url_for('main.login'))
    return render_template('register.html', form=form)

@main.route('/logout')
def logout():
    logout_user()
    return redirect(url_for('main.index'))

@main.route('/dashboard')
@login_required
@limiter.limit(lambda: current_app.config.get('RATELIMIT_DEFAULT', '200 per day'))
def dashboard():
    urls = URL.query.filter_by(user_id=current_user.id).order_by(URL.created_at.desc()).all()
    # Add full URL for display
    for u in urls:
        u.full_short_url = f"https://{current_app.config['BASE_DOMAIN']}/{u.short_code}"
    return render_template('dashboard.html', urls=urls)

@main.route('/regenerate-api-key', methods=['POST'])
@login_required
def regenerate_api_key():
    current_user.api_key = str(uuid.uuid4())
    db.session.commit()
    flash('API key has been regenerated.', 'success')
    return redirect(url_for('main.dashboard'))

@main.route('/export-csv')
@login_required
def export_csv():
    urls = URL.query.filter_by(user_id=current_user.id).all()
    
    output = io.StringIO()
    writer = csv.writer(output)

    # Header with security warning
    writer.writerow(['# Short Link Export - RedRx'])
    writer.writerow(['Short Code', 'Long URL', 'Clicks', 'Created At', 'Last Accessed', 'Expires At'])
    
    for u in urls:
        # CSV Injection protection: Escape values starting with special chars
        l_url = u.long_url
        if l_url and l_url[0] in ('=', '+', '-', '@'):
            l_url = "'" + l_url

        writer.writerow([
            u.short_code,
            l_url,
            u.clicks,
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
@limiter.limit("10 per minute")
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
        if not is_safe_url(form.long_url.data):
            flash("That destination URL is blocked or invalid.", 'danger')
            return render_template('edit_url.html', form=form, short_code=short_code)
        if form.ios_target_url.data and not is_safe_url(form.ios_target_url.data):
            flash("iOS target URL is blocked or invalid.", 'danger')
            return render_template('edit_url.html', form=form, short_code=short_code)
        if form.android_target_url.data and not is_safe_url(form.android_target_url.data):
            flash("Android target URL is blocked or invalid.", 'danger')
            return render_template('edit_url.html', form=form, short_code=short_code)

        url_entry.long_url = form.long_url.data
        url_entry.ios_target_url = form.ios_target_url.data
        url_entry.android_target_url = form.android_target_url.data
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
@limiter.limit("20 per minute")
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
@limiter.limit("30 per minute")
def qr_download(short_code):
    short_url = f"https://{current_app.config['BASE_DOMAIN']}/{short_code}"
    
    # Generate simple QR for download (black/white usually best for raw download, or use defaults)
    img_buffer = generate_qr(short_url)
    return send_file(img_buffer, mimetype='image/png', as_attachment=False, download_name=f'{short_code}.png')

@main.errorhandler(404)
def not_found(e):
    return render_template('404.html'), 404

@main.errorhandler(403)
def forbidden(e):
    return render_template('403.html'), 403

@main.errorhandler(410)
def gone(e):
    return render_template('410.html'), 410

@main.errorhandler(500)
def internal_error(e):
    return render_template('500.html'), 500

@main.errorhandler(429)
def ratelimit_handler(e):
    # Increment Prometheus Counter
    ratelimit_hits_total.inc()
    
    client_ip = get_client_ip(request)
    client_country = get_geo_info(client_ip, request)
    return render_template('429.html', client_ip=client_ip, client_country=client_country), 429

@main.route('/robots.txt')
def robots():
    if not current_app.config.get('ENABLE_SEO'):
        abort(404)
    return render_template('robots.txt'), 200, {'Content-Type': 'text/plain'}

@main.route('/sitemap.xml')
def sitemap():
    if not current_app.config.get('ENABLE_SEO'):
        abort(404)
    return render_template('sitemap.xml'), 200, {'Content-Type': 'application/xml'}

@main.route('/api-docs')
@limiter.limit("30 per minute")
def api_docs():
    return render_template('api_docs.html')

@main.route('/data-usage')
@limiter.limit("30 per minute")
def data_usage():
    return render_template('data_usage.html')

@main.route('/terms')
@limiter.limit("30 per minute")
def terms():
    return render_template('terms.html')

@main.route('/<short_code>', methods=['GET', 'POST'])
@limiter.limit(lambda: current_app.config.get('RATELIMIT_REDIRECT', '100 per minute'))
def redirect_to_url(short_code):
    url_entry = URL.query.filter_by(short_code=short_code).first_or_404()

    if not url_entry.is_active():
        abort(410)

    # 1. Check for Password Protection
    if url_entry.password_hash:
        # Check if already authorized in session
        session_key = f'auth_{short_code}'
        if not session.get(session_key):
            form = LinkPasswordForm()
            if form.validate_on_submit():
                if check_password_hash(url_entry.password_hash, form.password.data):
                    session[session_key] = True
                    # Continue to preview/redirect
                else:
                    flash('Invalid password.', 'danger')
                    return render_template('redirect.html', form=form, url=url_entry, password_required=True)
            else:
                return render_template('redirect.html', form=form, url=url_entry, password_required=True)

    # 2. Statistics and Analytics
    if url_entry.stats_enabled:
        user_agent = parse(request.user_agent.string)
        ip_address = get_client_ip(request)
        country = get_geo_info(ip_address, request)

        click = Click(
            url_id=url_entry.id,
            ip_address=ip_address,
            country=country,
            browser=user_agent.browser.family,
            platform=user_agent.os.family,
            referrer=request.referrer
        )
        db.session.add(click)
        url_entry.clicks += 1
        url_entry.last_accessed_at = datetime.datetime.now(datetime.timezone.utc)
        db.session.commit()

        # Increment Prometheus Counter
        redirections_total.inc()

    # 3. Handle Targeted Redirection (Device-Specific)
    target_url = url_entry.long_url
    user_agent_str = request.user_agent.string.lower()

    if 'iphone' in user_agent_str or 'ipad' in user_agent_str:
        if url_entry.ios_target_url:
            target_url = url_entry.ios_target_url
    elif 'android' in user_agent_str:
        if url_entry.android_target_url:
            target_url = url_entry.android_target_url

    # 4. Preview Mode handling
    if url_entry.preview_mode:
        # If user comes from the preview page itself via "Continue" button
        if request.args.get('preview') == 'false':
            return redirect(target_url)

        # Show preview page
        qr_code_data = generate_qr(f"https://{current_app.config['BASE_DOMAIN']}/{short_code}")
        import base64
        qr_code_base64 = base64.b64encode(qr_code_data.getvalue()).decode()

        return render_template('preview.html',
                             url=url_entry,
                             target_url=target_url,
                             qr_code=qr_code_base64)

    return redirect(target_url)
