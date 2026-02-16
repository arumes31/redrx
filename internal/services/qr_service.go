package services

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"

	"github.com/skip2/go-qrcode"
)

type QROptions struct {
	Content string
	Size    int
	FgColor string // Hex code e.g. "#000000"
	BgColor string // Hex code e.g. "#FFFFFF"
	Logo    image.Image
}

func GenerateQRCode(opts QROptions) (string, []byte, error) {
	qr, err := qrcode.New(opts.Content, qrcode.Highest) // Highest level for logo tolerance
	if err != nil {
		return "", nil, err
	}

	// Parse Colors
	qr.ForegroundColor = parseHexColor(opts.FgColor, color.Black)
	qr.BackgroundColor = parseHexColor(opts.BgColor, color.White)

	// Generate Image
	img := qr.Image(opts.Size)

	// Embed Logo if present
	if opts.Logo != nil {
		img = embedLogo(img, opts.Logo)
	}

	// Encode to PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", nil, err
	}

	pngBytes := buf.Bytes()
	base64Str := base64.StdEncoding.EncodeToString(pngBytes)
	return base64Str, pngBytes, nil
}

// GenerateQRCodeSVG generates an SVG string for the QR code.
// Note: Logo embedding is currently NOT supported for SVG to keep it simple.
func GenerateQRCodeSVG(opts QROptions) (string, error) {
	qr, err := qrcode.New(opts.Content, qrcode.Highest)
	if err != nil {
		return "", err
	}

	qr.DisableBorder = true
	bitmap := qr.Bitmap()
	size := len(bitmap)

	// SVG Header
	var sb strings.Builder
	// ViewBox matches module count
	sb.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges">`, size, size))

	// Background
	sb.WriteString(fmt.Sprintf(`<rect width="100%%" height="100%%" fill="%s"/>`, opts.BgColor))

	// Foreground Modules
	sb.WriteString(fmt.Sprintf(`<path fill="%s" d="`, opts.FgColor))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if bitmap[y][x] {
				sb.WriteString(fmt.Sprintf("M%d %dh1v1h-1z ", x, y))
			}
		}
	}
	sb.WriteString(`"/>`)

	sb.WriteString("</svg>")
	return sb.String(), nil
}

func parseHexColor(s string, defaultColor color.Color) color.Color {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return defaultColor
	}

	hexToByte := func(c byte) byte {
		if c >= '0' && c <= '9' {
			return c - '0'
		}
		if c >= 'a' && c <= 'f' {
			return c - 'a' + 10
		}
		if c >= 'A' && c <= 'F' {
			return c - 'A' + 10
		}
		return 0
	}

	r := (hexToByte(s[0]) << 4) + hexToByte(s[1])
	g := (hexToByte(s[2]) << 4) + hexToByte(s[3])
	b := (hexToByte(s[4]) << 4) + hexToByte(s[5])

	return color.RGBA{R: r, G: g, B: b, A: 255}
}

// Nearest-neighbor resize
func resizeImage(img image.Image, newWidth, newHeight int) image.Image {
	r := image.Rect(0, 0, newWidth, newHeight)
	dst := image.NewRGBA(r)

	xRatio := float64(img.Bounds().Dx()) / float64(newWidth)
	yRatio := float64(img.Bounds().Dy()) / float64(newHeight)

	for y := 0; y < newHeight; y++ {
		for x := 0; x < newWidth; x++ {
			srcX := int(float64(x) * xRatio)
			srcY := int(float64(y) * yRatio)
			dst.Set(x, y, img.At(img.Bounds().Min.X+srcX, img.Bounds().Min.Y+srcY))
		}
	}
	return dst
}

func embedLogo(src image.Image, logo image.Image) image.Image {
	b := src.Bounds()
	m := image.NewRGBA(b)

	draw.Draw(m, b, src, image.Point{}, draw.Src)

	// Calculate target logo size (e.g. 20% of QR width)
	targetW := b.Dx() / 5
	targetH := b.Dy() / 5

	resizedLogo := resizeImage(logo, targetW, targetH)

	offset := image.Pt(
		(b.Dx()-targetW)/2,
		(b.Dy()-targetH)/2,
	)

	draw.Draw(m, image.Rect(offset.X, offset.Y, offset.X+targetW, offset.Y+targetH), resizedLogo, image.Point{}, draw.Over)
	return m
}
