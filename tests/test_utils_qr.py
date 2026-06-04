import io
import base64
from PIL import Image
from app.utils import generate_qr, get_qr_data_url

def test_generate_qr_basic():
    """Test basic QR generation with valid parameters."""
    buffer = generate_qr("https://example.com")
    assert isinstance(buffer, io.BytesIO)
    img = Image.open(buffer)
    assert img.format == 'PNG'

def test_generate_qr_invalid_color_fallback():
    """Test QR generation fallback when invalid colors are provided."""
    # This should trigger the ValueError in app/utils.py and fall back to black/white
    buffer = generate_qr("https://example.com", color="invalid_color", bg="another_invalid_one")
    assert isinstance(buffer, io.BytesIO)
    img = Image.open(buffer)
    assert img.format == 'PNG'
    # Verification that it didn't crash is the primary goal here

def test_get_qr_data_url_basic():
    """Test basic QR data URL generation."""
    data_url = get_qr_data_url("https://example.com")
    assert isinstance(data_url, str)
    # It returns only the base64 string, not the full data:image/png;base64,...
    # (based on get_qr_data_url implementation in app/utils.py)
    decoded = base64.b64decode(data_url)
    img = Image.open(io.BytesIO(decoded))
    assert img.format == 'PNG'

def test_get_qr_data_url_invalid_color_fallback():
    """Test QR data URL generation fallback when invalid colors are provided."""
    data_url = get_qr_data_url("https://example.com", color="not-a-color")
    assert isinstance(data_url, str)
    decoded = base64.b64decode(data_url)
    img = Image.open(io.BytesIO(decoded))
    assert img.format == 'PNG'

def test_generate_qr_with_logo():
    """Test QR generation with a logo image."""
    logo = Image.new('RGBA', (100, 100), color='red')
    buffer = generate_qr("https://example.com", logo_img=logo)
    assert isinstance(buffer, io.BytesIO)
    img = Image.open(buffer)
    assert img.format == 'PNG'

def test_generate_qr_type_error_fallback():
    """Test QR generation fallback when invalid types are provided for colors."""
    # This should trigger the TypeError in app/utils.py and fall back to black/white
    buffer = generate_qr("https://example.com", color=[1, 2, 3])
    assert isinstance(buffer, io.BytesIO)
    img = Image.open(buffer)
    assert img.format == 'PNG'
