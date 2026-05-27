import io
import pytest
from PIL import Image
from app.utils import generate_qr

def test_generate_qr_valid_colors():
    """Test generating a QR code with valid colors."""
    data = "https://example.com"
    img_buffer = generate_qr(data, color='blue', bg='yellow')

    # Verify it's a valid PNG
    img = Image.open(img_buffer)
    assert img.format == 'PNG'
    assert img.mode == 'RGB'

def test_generate_qr_invalid_fill_color_fallback():
    """Test generating a QR code with an invalid fill color, triggering fallback."""
    data = "https://example.com"
    # PIL/qrcode should raise ValueError for 'not-a-color'
    img_buffer = generate_qr(data, color='not-a-color', bg='white')

    # Verify it still returns a valid image (fallback worked)
    img = Image.open(img_buffer)
    assert img.format == 'PNG'
    assert img.mode == 'RGB'

def test_generate_qr_invalid_bg_color_fallback():
    """Test generating a QR code with an invalid background color, triggering fallback."""
    data = "https://example.com"
    img_buffer = generate_qr(data, color='black', bg='invalid-bg-color')

    # Verify it still returns a valid image (fallback worked)
    img = Image.open(img_buffer)
    assert img.format == 'PNG'
    assert img.mode == 'RGB'

def test_generate_qr_with_logo():
    """Test generating a QR code with a logo image."""
    data = "https://example.com"

    # Create a small dummy logo image
    logo_size = (50, 50)
    logo_img = Image.new('RGBA', logo_size, color='red')

    img_buffer = generate_qr(data, logo_img=logo_img)

    # Verify it's a valid PNG
    img = Image.open(img_buffer)
    assert img.format == 'PNG'
    assert img.mode == 'RGB'
