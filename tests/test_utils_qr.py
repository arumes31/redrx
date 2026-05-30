import io
from PIL import Image
from app.utils import generate_qr

def test_generate_qr_success():
    """Test QR generation with default colors."""
    data = "https://example.com"
    img_buffer = generate_qr(data)
    assert isinstance(img_buffer, io.BytesIO)

    img = Image.open(img_buffer)
    assert img.format == 'PNG'
    assert img.mode == 'RGB'

def test_generate_qr_custom_colors():
    """Test QR generation with valid custom colors."""
    data = "https://example.com"
    img_buffer = generate_qr(data, color='red', bg='blue')
    assert isinstance(img_buffer, io.BytesIO)

    img = Image.open(img_buffer)
    assert img.format == 'PNG'
    assert img.mode == 'RGB'

def test_generate_qr_invalid_color_fallback():
    """Test QR generation fallback when invalid colors are provided."""
    data = "https://example.com"
    # PIL handles many strings, but 'not-a-real-color' should trigger ValueError
    # in qrcode/PIL's make_image or similar internal calls.
    # Passing something definitely invalid to force the except block.
    img_buffer = generate_qr(data, color='invalid-color-name-that-does-not-exist')
    assert isinstance(img_buffer, io.BytesIO)

    img = Image.open(img_buffer)
    assert img.format == 'PNG'
    assert img.mode == 'RGB'

def test_generate_qr_with_logo():
    """Test QR generation with a logo overlay."""
    data = "https://example.com"
    logo = Image.new('RGBA', (100, 100), color='green')
    img_buffer = generate_qr(data, logo_img=logo)
    assert isinstance(img_buffer, io.BytesIO)

    img = Image.open(img_buffer)
    assert img.format == 'PNG'
    assert img.mode == 'RGB'
