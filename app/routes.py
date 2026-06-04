import io
import csv
import datetime
import uuid
from flask import (
    Blueprint,
    render_template,
    request,
    redirect,
    url_for,
    flash,
    abort,
    send_file,
    current_app,
    session,
    jsonify,
)
from flask_login import login_user, logout_user, current_user, login_required
from app import limiter
from werkzeug.security import generate_password_hash, check_password_hash
from PIL import Image
from user_agents import parse
from urllib.parse import urlparse
from sqlalchemy import func, text
from prometheus_client import Counter
from app.models import db, URL, User, Click
from app.forms import (
    ShortenURLForm,
    LoginForm,
    RegisterForm,
    LinkPasswordForm,
    EditURLForm,
)
from app.utils import (
    generate_short_code,
    get_qr_data_url,
    generate_qr,
    select_rotate_target,
    get_geo_info,
    is_safe_url,
    get_client_ip,
    _get_redis_client,
    is_safe_redirect_url,
    get_blocked_domains,
    sanitize_csv_field,
)

# Custom Metrics
shortened_links_total = Counter(
    "redrx_shortened_links_total", "Total number of shortened links created"
)
redirections_total = Counter(
    "redrx_redirections_total", "Total number of link redirections"
)
ratelimit_hits_total = Counter(
    "redrx_ratelimit_hits_total", "Total number of requests hitting the rate limit"
)


main = Blueprint("main", __name__)


@main.route("/health")
@limiter.limit(lambda: current_app.config.get("RATELIMIT_HEALTH", "10 per minute"))
def health_check():
    health = {"status": "healthy", "checks": {}}

    # 1. Database Check
    try:
        db.session.execute(text("SELECT 1"))
        health["checks"]["database"] = "ok"
    except Exception as e:
        health["status"] = "unhealthy"
        health["checks"]["database"] = f"error: {str(e)}"

    # 2. Redis Check
    try:
        r = _get_redis_client()
        if r and r.ping():
            health["checks"]["redis"] = "ok"
        else:
            health["checks"]["redis"] = "disconnected"
            health["status"] = "degraded"
    except Exception as e:
        health["checks"]["redis"] = f"error: {str(e)}"
        health["status"] = "degraded"

    code = 200 if health["status"] == "healthy" else 503
    return jsonify(health), code


def _handle_short_code(form):
    custom_code = (
        form.custom_code.data.strip().upper() if form.custom_code.data else None
    )
    if custom_code:
        if URL.query.filter_by(short_code=custom_code).first():
            return None, f"Code '{custom_code}' is already taken."
        return custom_code, None
    else:
        length = form.code_length.data or current_app.config["SHORT_CODE_LENGTH"]
        short_code = generate_short_code(length)
        while URL.query.filter_by(short_code=short_code).first():
            short_code = generate_short_code(length)
        return short_code, None


def _validate_additional_urls(form):
    rotate_list = (
        [u.strip() for u in form.rotate_targets.data.split(",") if u.strip()]
        if form.rotate_targets.data
        else None
    )
    if rotate_list:
        if len(rotate_list) > 50:
            return None, "Maximum 50 rotate targets allowed."
        if not all(is_safe_url(u) for u in rotate_list):
            return None, "One or more rotate target URLs are blocked or invalid."
    if form.ios_target_url.data and not is_safe_url(form.ios_target_url.data):
        return None, "iOS target URL is blocked or invalid."
    if form.android_target_url.data and not is_safe_url(form.android_target_url.data):
        return None, "Android target URL is blocked or invalid."
    return rotate_list, None


def _process_url_timestamps(form):
    start_at = None
    if form.start_date.data and form.start_time.data:
        start_at = datetime.datetime.combine(form.start_date.data, form.start_time.data)
    end_at = None
    if form.end_date.data and form.end_time.data:
        end_at = datetime.datetime.combine(form.end_date.data, form.end_time.data)

    if start_at and end_at and end_at <= start_at:
        return start_at, end_at, None, "End time must be after start time"

    expires_at = None
    if form.expiry_hours.data is not None:
        if form.expiry_hours.data == 0 or form.expiry_hours.data > 8760:
            if not current_user.is_authenticated:
                return (
                    None,
                    None,
                    None,
                    "Please log in to create links longer than 1 year or permanent links.",
                )

            if form.expiry_hours.data == 0:
                expires_at = None
            else:
                expires_at = datetime.datetime.now(
                    datetime.timezone.utc
                ) + datetime.timedelta(hours=form.expiry_hours.data)
        else:
            expires_at = datetime.datetime.now(
                datetime.timezone.utc
            ) + datetime.timedelta(hours=form.expiry_hours.data)
    return start_at, end_at, expires_at, None


def _generate_qr_data_for_form(form, short_url):
    logo_img = None
    if form.logo_file.data:
        try:
            logo_img = Image.open(form.logo_file.data)
        except Exception:
            flash("Invalid Logo Image", "warning")

    return get_qr_data_url(
        short_url, color=form.qr_color.data, bg=form.qr_bg.data, logo_img=logo_img
    )


@main.route("/", methods=["GET", "POST"])
@limiter.limit(
    lambda: current_app.config.get("RATELIMIT_CREATE", "10 per minute"),
    methods=["POST"],
)
@limiter.limit(
    lambda: current_app.config.get("RATELIMIT_DEFAULT", "200 per day"), methods=["GET"]
)
def index():
    form = ShortenURLForm()

    short_url = None
    qr_data = None
    stats_url = None

    if form.validate_on_submit():
        if (
            current_app.config.get("DISABLE_ANONYMOUS_CREATE")
            and not current_user.is_authenticated
        ):
            flash("Please log in to shorten URLs.", "warning")
            return redirect(url_for("main.login"))

        long_url = form.long_url.data
        if not is_safe_url(long_url):
            flash("That destination URL is blocked for safety reasons.", "danger")
            return render_template("index.html", form=form)

        # Handle short code
        short_code, error = _handle_short_code(form)
        if error:
            flash(error, "danger")
            return render_template("index.html", form=form)

        # Validate additional URLs
        rotate_list, error = _validate_additional_urls(form)
        if error:
            flash(error, "danger")
            return render_template("index.html", form=form)

        # Process timestamps and expiry
        start_at, end_at, expires_at, error = _process_url_timestamps(form)
        if error:
            flash(error, "warning")
            return render_template("index.html", form=form)

        # Create Record
        new_url = URL(
            user_id=current_user.id if current_user.is_authenticated else None,
            short_code=short_code,
            long_url=long_url,
            rotate_targets=rotate_list,
            ios_target_url=form.ios_target_url.data,
            android_target_url=form.android_target_url.data,
            password_hash=generate_password_hash(form.password.data)
            if form.password.data
            else None,
            preview_mode=form.preview_mode.data,
            stats_enabled=form.stats_enabled.data,
            start_at=start_at,
            end_at=end_at,
            expires_at=expires_at,
        )
        db.session.add(new_url)
        db.session.commit()

        shortened_links_total.inc()

        # Post-creation info
        short_url = f"https://{current_app.config['BASE_DOMAIN']}/{short_code}"
        qr_data = _generate_qr_data_for_form(form, short_url)
        stats_url = url_for("main.stats", short_code=short_code, _external=True)

        flash("URL Shortened Successfully!", "success")

    return render_template(
        "index.html",
        form=form,
        short_url=short_url,
        qr_data=qr_data,
        stats_url=stats_url,
    )


@main.route("/<short_code>")
@limiter.limit(lambda: current_app.config.get("RATELIMIT_REDIRECT", "100 per minute"))
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
            return redirect(url_for("main.link_password_auth", short_code=short_code))

    ua_string = request.headers.get("User-Agent")
    user_agent = parse(ua_string)
    cached_domains = get_blocked_domains()

    target_url = _get_target_url(url_entry, user_agent, cached_domains)

    if not is_safe_url(target_url, cached_domains):
        abort(403)

    # Update last accessed
    url_entry.last_accessed_at = datetime.datetime.now(datetime.timezone.utc)
    db.session.commit()

    # Increment Prometheus Counter
    redirections_total.inc()

    # Stats
    if url_entry.stats_enabled:
        client_ip = get_client_ip(request)
        _record_click(url_entry, user_agent, client_ip)

    # If preview mode is enabled
    if url_entry.preview_mode:
        return render_template(
            "preview.html", target_url=target_url, short_code=short_code
        )

    return render_template("redirect.html", target_url=target_url)


@main.route("/link-auth/<short_code>", methods=["GET", "POST"])
@limiter.limit(lambda: current_app.config.get("RATELIMIT_AUTH", "10 per minute"))
def link_password_auth(short_code):
    url_entry = URL.query.filter_by(short_code=short_code).first_or_404()
    form = LinkPasswordForm()

    if form.validate_on_submit():
        if check_password_hash(url_entry.password_hash, form.password.data):
            session[f"auth_{short_code}"] = True
            return redirect(url_for("main.redirect_to_url", short_code=short_code))
        else:
            flash("Invalid Password", "danger")

    return render_template("login.html", form=form, short_code=short_code)


@main.route("/register", methods=["GET", "POST"])
@limiter.limit(
    lambda: current_app.config.get("RATELIMIT_REGISTER", "5 per hour")
)  # Prevent spam accounts
def register():
    if current_app.config.get("DISABLE_REGISTRATION"):
        flash("Registration is currently disabled.", "info")
        return redirect(url_for("main.index"))
    if current_user.is_authenticated:
        return redirect(url_for("main.index"))
    form = RegisterForm()
    if form.validate_on_submit():
        hashed_password = generate_password_hash(form.password.data)
        api_key = str(uuid.uuid4())
        user = User(
            username=form.username.data,
            email=form.email.data,
            password_hash=hashed_password,
            api_key=api_key,
        )
        db.session.add(user)
        db.session.commit()
        flash("Your account has been created! You can now log in.", "success")
        return redirect(url_for("main.login"))
    return render_template("register.html", form=form)


@main.route("/login", methods=["GET", "POST"])
@limiter.limit(
    lambda: current_app.config.get("RATELIMIT_LOGIN", "10 per minute")
)  # Prevent brute force
def login():
    if current_user.is_authenticated:
        return redirect(url_for("main.index"))
    form = LoginForm()
    if form.validate_on_submit():
        user = User.query.filter(
            (User.username == form.username.data) | (User.email == form.username.data)
        ).first()
        if user and check_password_hash(user.password_hash, form.password.data):
            login_user(user)
            next_page = request.args.get("next")
            if next_page and is_safe_redirect_url(next_page):
                return redirect(next_page)
            return redirect(url_for("main.index"))
        else:
            flash(
                "Login Unsuccessful. Please check username/email and password", "danger"
            )
    return render_template("login_user.html", form=form)


@main.route("/logout")
def logout():
    logout_user()
    return redirect(url_for("main.index"))


@main.route("/dashboard")
@login_required
@limiter.limit("60 per minute")  # High limit for dashboard usage
def dashboard():
    page = request.args.get("page", 1, type=int)
    per_page = 10

    # Efficient stats via SQL aggregations
    stats_query = (
        db.session.query(
            func.count(URL.id).label("total_links"),
            func.sum(URL.clicks_count).label("total_clicks"),
        )
        .filter(URL.user_id == current_user.id)
        .first()
    )

    active_links = URL.query.filter_by(user_id=current_user.id, is_enabled=True).count()

    # Pagination
    pagination = (
        URL.query.filter_by(user_id=current_user.id)
        .order_by(URL.created_at.desc())
        .paginate(page=page, per_page=per_page, error_out=False)
    )

    urls = pagination.items

    # Top performer (separate query, efficient)
    top_performer = (
        URL.query.filter_by(user_id=current_user.id)
        .order_by(URL.clicks_count.desc())
        .first()
    )

    stats = {
        "total_links": stats_query.total_links or 0,
        "total_clicks": stats_query.total_clicks or 0,
        "active_links": active_links,
        "top_performer": top_performer,
    }

    return render_template(
        "dashboard.html", urls=urls, stats=stats, pagination=pagination
    )


@main.route("/regenerate-api-key", methods=["POST"])
@login_required
@limiter.limit(lambda: current_app.config.get("RATELIMIT_AUTH", "5 per hour"))
def regenerate_api_key():
    current_user.api_key = str(uuid.uuid4())
    db.session.commit()
    flash("API Key regenerated successfully.", "success")
    return redirect(url_for("main.dashboard"))


@main.route("/toggle-status/<short_code>", methods=["POST"])
@login_required
@limiter.limit("60 per minute")
def toggle_status(short_code):
    url_entry = URL.query.filter_by(short_code=short_code).first_or_404()
    if url_entry.user_id != current_user.id:
        abort(403)
    url_entry.is_enabled = not url_entry.is_enabled
    db.session.commit()
    return jsonify({"status": "success", "is_enabled": url_entry.is_enabled})


@main.route("/export-links")
@login_required
@limiter.limit("5 per minute")
def export_links():
    urls = URL.query.filter_by(user_id=current_user.id).all()

    output = io.StringIO()
    writer = csv.writer(output)
    writer.writerow(
        [
            "Short Code",
            "Long URL",
            "Clicks",
            "Created At",
            "Last Accessed",
            "Expires At",
        ]
    )

    for u in urls:
        writer.writerow(
            [
                sanitize_csv_field(u.short_code),
                sanitize_csv_field(u.long_url),
                sanitize_csv_field(u.clicks_count),
                sanitize_csv_field(u.created_at.isoformat()),
                sanitize_csv_field(
                    u.last_accessed_at.isoformat() if u.last_accessed_at else "Never"
                ),
                sanitize_csv_field(
                    u.expires_at.isoformat() if u.expires_at else "Never"
                ),
            ]
        )

    output.seek(0)
    return send_file(
        io.BytesIO(output.getvalue().encode("utf-8")),
        mimetype="text/csv",
        as_attachment=True,
        download_name="my_links.csv",
    )


@main.route("/bulk-delete", methods=["POST"])
@login_required
@limiter.limit("10 per minute")
def bulk_delete():
    ids = request.form.getlist("link_ids")
    if not ids:
        flash("No links selected.", "warning")
        return redirect(url_for("main.dashboard"))

    URL.query.filter(URL.id.in_(ids), URL.user_id == current_user.id).delete(
        synchronize_session=False
    )
    db.session.commit()
    flash(f"Successfully deleted {len(ids)} links.", "info")
    return redirect(url_for("main.dashboard"))


@main.route("/edit/<short_code>", methods=["GET", "POST"])
@login_required
def edit_url(short_code):
    url_entry = URL.query.filter_by(short_code=short_code).first_or_404()

    if url_entry.user_id != current_user.id:
        abort(403)

    form = EditURLForm(obj=url_entry)

    if request.method == "GET" and url_entry.expires_at:
        now = datetime.datetime.now(datetime.timezone.utc).replace(tzinfo=None)
        diff = url_entry.expires_at - now
        hours = int(diff.total_seconds() / 3600)
        form.expiry_hours.data = max(0, hours)

    if form.validate_on_submit():
        if not is_safe_url(form.long_url.data):
            flash("That destination URL is blocked or invalid.", "danger")
            return render_template("edit_url.html", form=form, short_code=short_code)
        if form.ios_target_url.data and not is_safe_url(form.ios_target_url.data):
            flash("iOS target URL is blocked or invalid.", "danger")
            return render_template("edit_url.html", form=form, short_code=short_code)
        if form.android_target_url.data and not is_safe_url(
            form.android_target_url.data
        ):
            flash("Android target URL is blocked or invalid.", "danger")
            return render_template("edit_url.html", form=form, short_code=short_code)

        url_entry.long_url = form.long_url.data
        url_entry.ios_target_url = form.ios_target_url.data
        url_entry.android_target_url = form.android_target_url.data
        url_entry.preview_mode = form.preview_mode.data
        url_entry.stats_enabled = form.stats_enabled.data

        if form.expiry_hours.data is not None:
            if form.expiry_hours.data == 0:
                url_entry.expires_at = None
            else:
                url_entry.expires_at = datetime.datetime.now(
                    datetime.timezone.utc
                ) + datetime.timedelta(hours=form.expiry_hours.data)

        db.session.commit()
        flash("Link updated successfully.", "success")
        return redirect(url_for("main.dashboard"))

    return render_template("edit_url.html", form=form, short_code=short_code)


@main.route("/delete/<short_code>", methods=["POST"])
@login_required
def delete_url(short_code):
    url_entry = URL.query.filter_by(short_code=short_code).first_or_404()

    if url_entry.user_id != current_user.id:
        abort(403)

    db.session.delete(url_entry)
    db.session.commit()
    flash("Link deleted successfully.", "info")
    return redirect(url_for("main.dashboard"))


def _get_target_url(url_entry, user_agent, cached_domains):
    """Determines the target URL based on device and rotation settings."""
    target_url = url_entry.long_url
    device_match = False

    if url_entry.ios_target_url and (user_agent.os.family == "iOS"):
        target_url = url_entry.ios_target_url
        device_match = True
    elif url_entry.android_target_url and (user_agent.os.family == "Android"):
        target_url = url_entry.android_target_url
        device_match = True

    if not device_match and url_entry.rotate_targets:
        safe_rotate_targets = [
            alt for alt in url_entry.rotate_targets if is_safe_url(alt, cached_domains)
        ]
        alt = select_rotate_target(safe_rotate_targets)
        if alt:
            target_url = alt

    return target_url


def _record_click(url_entry, user_agent, client_ip):
    """Records a click with detailed statistics."""
    url_entry.clicks_count += 1
    masked_ip = _anonymize_ip(client_ip)

    new_click = Click(
        url_id=url_entry.id,
        ip_address=masked_ip,
        country=get_geo_info(client_ip, request),
        browser=user_agent.browser.family,
        platform=user_agent.os.family,
        referrer=request.referrer or "Direct",
    )
    db.session.add(new_click)
    db.session.commit()


def _get_time_range_config(range_type, now):
    import datetime

    # Configuration for different time ranges: (timedelta, days_for_avg, sqlite_fmt, pg_fmt, step_unit, step_count, label_fmt)
    configs = {
        "24h": (
            datetime.timedelta(hours=24),
            1,
            "%H:00",
            "HH24:00",
            "hours",
            24,
            "%H:00",
        ),
        "7d": (
            datetime.timedelta(days=7),
            7,
            "%Y-%m-%d",
            "YYYY-MM-DD",
            "days",
            7,
            "%Y-%m-%d",
        ),
        "30d": (
            datetime.timedelta(days=30),
            30,
            "%Y-%m-%d",
            "YYYY-MM-DD",
            "days",
            30,
            "%Y-%m-%d",
        ),
    }
    delta, days, sqlite_fmt, pg_fmt, step_unit, step_count, label_fmt = configs.get(
        range_type, configs["30d"]
    )

    cutoff = now - delta
    time_data = {
        (now - datetime.timedelta(**{step_unit: i})).strftime(label_fmt): 0
        for i in range(step_count, -1, -1)
    }

    return cutoff, sqlite_fmt, pg_fmt, time_data, days


def _anonymize_ip(ip):
    if not ip:
        return "Unknown"
    if "." in ip:  # IPv4
        parts = ip.split(".")
        if len(parts) == 4:
            return f"{parts[0]}.{parts[1]}.xxx.xxx"
    elif ":" in ip:  # IPv6
        v6_parts = ip.split(":")
        if len(v6_parts) >= 2:
            return f"{v6_parts[0]}:{v6_parts[1]}:xxxx:xxxx"
    return "xxxx"


def _get_relative_time(ts, now):
    import datetime

    ts_aware = ts.replace(tzinfo=datetime.timezone.utc) if ts.tzinfo is None else ts
    diff = now - ts_aware
    if diff.days > 0:
        return f"{diff.days}d ago"
    elif diff.seconds >= 3600:
        return f"{diff.seconds // 3600}h ago"
    elif diff.seconds >= 60:
        return f"{diff.seconds // 60}m ago"
    return "Just now"


def _get_time_series_stats(url_id, cutoff, sqlite_format, pg_format, time_data):
    dialect = db.engine.dialect.name
    format_str = sqlite_format if dialect == "sqlite" else pg_format
    # Select the appropriate time formatting function based on DB dialect
    time_func = func.strftime if dialect == "sqlite" else func.to_char
    time_col = time_func(format_str, Click.timestamp)

    time_counts = (
        db.session.query(time_col, func.count(Click.id))
        .filter(Click.url_id == url_id, Click.timestamp >= cutoff)
        .group_by(time_col)
        .all()
    )

    for key, count in time_counts:
        if key in time_data:
            time_data[key] = count
    return time_data


def _get_grouped_stats(url_id, cutoff, column):
    counts = (
        db.session.query(column, func.count(Click.id))
        .filter(Click.url_id == url_id, Click.timestamp >= cutoff)
        .group_by(column)
        .all()
    )
    return {val or "Unknown": count for val, count in counts}


def _get_referrer_stats(url_id, cutoff):
    ref_counts = (
        db.session.query(Click.referrer, func.count(Click.id))
        .filter(Click.url_id == url_id, Click.timestamp >= cutoff)
        .group_by(Click.referrer)
        .all()
    )
    referrer_data = {}
    for ref, count in ref_counts:
        label = ref or "Direct"
        if "://" in label:
            label = urlparse(label).netloc or label
        referrer_data[label] = referrer_data.get(label, 0) + count
    return referrer_data


def _check_stats_access(url_entry):
    current_user_id = current_user.id if current_user.is_authenticated else None
    if url_entry.user_id is None or url_entry.user_id != current_user_id:
        abort(403)


def _prepare_recent_clicks(url_id, now):
    recent_clicks = (
        Click.query.filter_by(url_id=url_id)
        .order_by(Click.timestamp.desc())
        .limit(10)
        .all()
    )
    for rc in recent_clicks:
        rc.relative_time = _get_relative_time(rc.timestamp, now)
        rc.ip_anonymized = _anonymize_ip(rc.ip_address)
    return recent_clicks


def _process_analytics(url_id, range_type, now):
    cutoff, sqlite_format, pg_format, time_data, days_in_range = _get_time_range_config(
        range_type, now
    )

    time_data = _get_time_series_stats(
        url_id, cutoff, sqlite_format, pg_format, time_data
    )
    country_data = _get_grouped_stats(url_id, cutoff, Click.country)
    browser_data = _get_grouped_stats(url_id, cutoff, Click.browser)
    platform_data = _get_grouped_stats(url_id, cutoff, Click.platform)
    referrer_data = _get_referrer_stats(url_id, cutoff)

    total_in_range = sum(time_data.values())
    avg_daily = round(total_in_range / days_in_range, 1)

    return (
        time_data,
        country_data,
        browser_data,
        platform_data,
        referrer_data,
        avg_daily,
    )


@main.route("/<short_code>/stats")
@limiter.limit("20 per minute")
def stats(short_code):
    url_entry = URL.query.filter_by(short_code=short_code).first_or_404()
    _check_stats_access(url_entry)

    range_type = request.args.get("range", "30d")
    now = datetime.datetime.now(datetime.timezone.utc)

    # Process Analytics
    time_data, country_data, browser_data, platform_data, referrer_data, avg_daily = (
        _process_analytics(url_entry.id, range_type, now)
    )

    # Recent activity (last 10)
    recent_clicks = _prepare_recent_clicks(url_entry.id, now)

    short_url = f"https://{current_app.config['BASE_DOMAIN']}/{short_code}"
    return render_template(
        "stats.html",
        url=url_entry,
        short_url=short_url,
        active=url_entry.is_active(),
        time_data=time_data,
        country_data=country_data,
        browser_data=browser_data,
        platform_data=platform_data,
        referrer_data=referrer_data,
        avg_daily=avg_daily,
        range_type=range_type,
        recent_clicks=recent_clicks,
    )


@main.route("/<short_code>/qr")
@limiter.limit("30 per minute")
def qr_download(short_code):
    URL.query.filter_by(short_code=short_code).first_or_404()
    short_url = f"https://{current_app.config['BASE_DOMAIN']}/{short_code}"

    # Generate simple QR for download (black/white usually best for raw download, or use defaults)
    img_buffer = generate_qr(short_url)
    return send_file(
        img_buffer,
        mimetype="image/png",
        as_attachment=False,
        download_name=f"{short_code}.png",
    )


@main.errorhandler(404)
def not_found(e):
    return render_template("404.html"), 404


@main.errorhandler(403)
def forbidden(e):
    return render_template("403.html"), 403


@main.errorhandler(410)
def gone(e):
    return render_template("410.html"), 410


@main.errorhandler(500)
def internal_error(e):
    return render_template("500.html"), 500


@main.errorhandler(429)
def ratelimit_handler(e):
    # Increment Prometheus Counter
    ratelimit_hits_total.inc()

    client_ip = get_client_ip(request)
    client_country = get_geo_info(client_ip, request)
    return render_template(
        "429.html", client_ip=client_ip, client_country=client_country
    ), 429


@main.route("/robots.txt")
def robots():
    if not current_app.config.get("ENABLE_SEO"):
        abort(404)
    return render_template("robots.txt"), 200, {"Content-Type": "text/plain"}


@main.route("/sitemap.xml")
def sitemap():
    if not current_app.config.get("ENABLE_SEO"):
        abort(404)
    return render_template("sitemap.xml"), 200, {"Content-Type": "application/xml"}


@main.route("/api-docs")
@limiter.limit("30 per minute")
def api_docs():
    return render_template("api_docs.html")


@main.route("/data-usage")
@limiter.limit("30 per minute")
def data_usage():
    return render_template("data_usage.html")


@main.route("/terms")
@limiter.limit("30 per minute")
def terms():
    return render_template("terms.html")
