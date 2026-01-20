import uuid
import qrcode
import io
import datetime
import base64
from PIL import Image

def generate_short_code(length=6):
    """Generates a random short code."""
    return str(uuid.uuid4())[:length].upper()

def generate_qr(data, color='black', bg='white', logo_img=None):
    """Generates a QR code image as a BytesIO object."""
    qr = qrcode.QRCode(version=1, box_size=10, border=5)
    qr.add_data(data)
    qr.make(fit=True)
    
    # Basic color validation/fallback could go here if needed, 
    # but PIL handles standard color names and hex codes well.
    try:
        img = qr.make_image(fill_color=color, back_color=bg).convert('RGB')
    except ValueError:
        # Fallback to defaults on error
        img = qr.make_image(fill_color='black', back_color='white').convert('RGB')

    if logo_img:
        # Ensure logo is compatible
        logo = logo_img.convert("RGBA")
        
        # Resize logo to max 20% of QR size
        max_size = (img.size[0] // 5, img.size[1] // 5)
        logo.thumbnail(max_size, Image.Resampling.LANCZOS)
        
        # Calculate position (center)
        pos = ((img.size[0] - logo.size[0]) // 2, (img.size[1] - logo.size[1]) // 2)
        
        # Create a mask for transparency if needed, but pasting directly works for RGBA on RGB
        img.paste(logo, pos, mask=logo)

    img_buffer = io.BytesIO()
    img.save(img_buffer, format='PNG')
    img_buffer.seek(0)
    return img_buffer

def select_ab_url(ab_urls):
    """Selects an alternate URL based on a simple rotation (hash of timestamp)."""
    if not ab_urls:
        return None
    # Using microsecond for more "random" feel on rapid refreshes
    idx = hash(str(datetime.datetime.now().microsecond)) % len(ab_urls)
    return ab_urls[idx]

def get_qr_data_url(data, color='black', bg='white', logo_img=None):
    """Returns a base64 encoded data URL for the QR code."""
    img_buffer = generate_qr(data, color, bg, logo_img)
    return base64.b64encode(img_buffer.read()).decode()
