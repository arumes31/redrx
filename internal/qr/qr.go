// Package qr renders QR codes, optionally with a logo composited in the centre.
package qr

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strconv"
	"strings"

	"github.com/skip2/go-qrcode"
	xdraw "golang.org/x/image/draw"

	// Registered so uploaded logos in these formats decode.
	_ "image/gif"
	_ "image/jpeg"
)

// pixelsPerModule matches the previous renderer's box_size of 10.
const pixelsPerModule = 10

// Options controls rendering.
type Options struct {
	// Foreground and Background accept CSS hex ("#rrggbb") or the handful of
	// colour names the old form defaulted to.
	Foreground string
	Background string
	// Logo, when non-nil, is composited over the centre of the code.
	Logo image.Image
}

// PNG renders content as a PNG image.
func PNG(content string, opts Options) ([]byte, error) {
	if content == "" {
		return nil, errors.New("qr: nothing to encode")
	}

	fg := parseColor(opts.Foreground, color.RGBA{0, 0, 0, 255})
	bg := parseColor(opts.Background, color.RGBA{255, 255, 255, 255})

	// Medium recovery matches the previous default and tolerates a logo
	// covering the centre.
	code, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return nil, fmt.Errorf("qr: encode: %w", err)
	}
	code.ForegroundColor = fg
	code.BackgroundColor = bg

	img := code.Image(-pixelsPerModule)

	if opts.Logo != nil {
		img = overlayLogo(img, opts.Logo)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("qr: encode png: %w", err)
	}
	return buf.Bytes(), nil
}

// DataURLPayload renders a code and returns it base64-encoded, ready to embed
// in a data: URL.
func DataURLPayload(content string, opts Options) (string, error) {
	raw, err := PNG(content, opts)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// overlayLogo scales the logo to a fifth of the code and draws it centred,
// respecting alpha so transparent logos do not punch out modules.
func overlayLogo(base image.Image, logo image.Image) image.Image {
	b := base.Bounds()
	out := image.NewRGBA(b)
	draw.Draw(out, b, base, b.Min, draw.Src)

	maxW, maxH := b.Dx()/5, b.Dy()/5
	if maxW < 1 || maxH < 1 {
		return out
	}

	lb := logo.Bounds()
	if lb.Dx() == 0 || lb.Dy() == 0 {
		return out
	}

	// Preserve the logo's aspect ratio inside the box, as thumbnail() did.
	scale := min(float64(maxW)/float64(lb.Dx()), float64(maxH)/float64(lb.Dy()))
	if scale > 1 {
		scale = 1
	}
	w := max(1, int(float64(lb.Dx())*scale))
	h := max(1, int(float64(lb.Dy())*scale))

	scaled := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), logo, lb, xdraw.Over, nil)

	offset := image.Pt(b.Min.X+(b.Dx()-w)/2, b.Min.Y+(b.Dy()-h)/2)
	draw.Draw(out, image.Rectangle{Min: offset, Max: offset.Add(image.Pt(w, h))},
		scaled, image.Point{}, draw.Over)
	return out
}

// namedColors covers the values the previous configuration defaulted to.
var namedColors = map[string]color.RGBA{
	"black":       {0, 0, 0, 255},
	"white":       {255, 255, 255, 255},
	"red":         {255, 0, 0, 255},
	"green":       {0, 128, 0, 255},
	"blue":        {0, 0, 255, 255},
	"yellow":      {255, 255, 0, 255},
	"cyan":        {0, 255, 255, 255},
	"magenta":     {255, 0, 255, 255},
	"gray":        {128, 128, 128, 255},
	"grey":        {128, 128, 128, 255},
	"transparent": {0, 0, 0, 0},
}

// parseColor accepts "#rgb", "#rrggbb", "#rrggbbaa" and the named colours,
// falling back to def for anything it cannot read.
func parseColor(s string, def color.RGBA) color.RGBA {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return def
	}
	if c, ok := namedColors[s]; ok {
		return c
	}
	if !strings.HasPrefix(s, "#") {
		return def
	}

	hex := s[1:]
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if len(hex) != 6 && len(hex) != 8 {
		return def
	}

	component := func(i int) (uint8, bool) {
		v, err := strconv.ParseUint(hex[i*2:i*2+2], 16, 8)
		if err != nil {
			return 0, false
		}
		return uint8(v), true
	}

	r, ok1 := component(0)
	g, ok2 := component(1)
	b, ok3 := component(2)
	if !ok1 || !ok2 || !ok3 {
		return def
	}
	a := uint8(255)
	if len(hex) == 8 {
		if v, ok := component(3); ok {
			a = v
		}
	}
	return color.RGBA{r, g, b, a}
}

// maxLogoPixels bounds the decoded size of an uploaded logo. The upload is
// already capped at 1 MB, but compressed size says nothing about decoded size:
// a ~40 KB PNG can declare 40000x40000, and image.Decode would allocate
// width*height*4 bytes — around 6 GB — before any of our code runs. That is an
// unrecoverable runtime OOM, not a panic, so the recover middleware cannot
// contain it. Reading the header first keeps the allocation bounded.
const maxLogoPixels = 4 << 20 // 4M pixels, e.g. 2048x2048

// DecodeLogo reads an uploaded image for use as a QR overlay.
func DecodeLogo(data []byte) (image.Image, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("qr: inspect logo: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, fmt.Errorf("qr: logo has no dimensions")
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxLogoPixels {
		return nil, fmt.Errorf("qr: logo is %dx%d, over the %d pixel limit",
			cfg.Width, cfg.Height, maxLogoPixels)
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("qr: decode logo: %w", err)
	}
	return img, nil
}
