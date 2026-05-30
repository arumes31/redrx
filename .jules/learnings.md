# Learnings

## Testing QR Code Generation
- The `qrcode` library's `make_image` method with `fill_color` and `back_color` delegates to PIL/Pillow for color resolution.
- Passing an invalid color name (e.g., 'invalid-color-name') triggers a `ValueError` in Pillow, which can be used to test fallback logic.
- When testing image generation, verifying the `io.BytesIO` output can be done by opening it with `PIL.Image.open` and checking properties like `format` and `mode`.

## Environment Management
- When running tests in this repository, `FLASK_DEBUG=true` must be set if `SECRET_KEY` is not provided in the environment, as `config.py` enforces a secret key in production mode.
- `pytest-cov` is a useful tool for verifying that specific error-handling blocks (like `except ValueError`) are actually exercised by tests.
