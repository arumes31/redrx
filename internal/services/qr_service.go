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

type QRService struct{}

func NewQRService() *QRService {
	return &QRService{}
}

func (s *QRService) GenerateQRCode(opts QROptions) (string, []byte, error) {
	qr, err := qrcode.New(opts.Content, qrcode.Highest)
	if err != nil {
		return "", nil, err
	}

	qr.ForegroundColor = s.parseHexColor(opts.FgColor, color.Black)
	qr.BackgroundColor = s.parseHexColor(opts.BgColor, color.White)

	img := qr.Image(opts.Size)

	if opts.Logo != nil {
		img = s.embedLogo(img, opts.Logo)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", nil, err
	}

	pngBytes := buf.Bytes()
	base64Str := base64.StdEncoding.EncodeToString(pngBytes)
	return base64Str, pngBytes, nil
}

func (s *QRService) GenerateQRCodeSVG(opts QROptions) (string, error) {
	qr, err := qrcode.New(opts.Content, qrcode.Highest)
	if err != nil {
		return "", err
	}

	qr.DisableBorder = true
	bitmap := qr.Bitmap()
	size := len(bitmap)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges">`, size, size))
	sb.WriteString(fmt.Sprintf(`<rect width="100%%" height="100%%" fill="%s"/>`, opts.BgColor))
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

func (s *QRService) parseHexColor(str string, defaultColor color.Color) color.Color {
	str = strings.TrimPrefix(str, "#")
	if len(str) != 6 {
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

	r := (hexToByte(str[0]) << 4) + hexToByte(str[1])
	g := (hexToByte(str[2]) << 4) + hexToByte(str[3])
	b := (hexToByte(str[4]) << 4) + hexToByte(str[5])

	return color.RGBA{R: r, G: g, B: b, A: 255}
}

func (s *QRService) resizeImage(img image.Image, newWidth, newHeight int) image.Image {
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

func (s *QRService) embedLogo(src image.Image, logo image.Image) image.Image {
	b := src.Bounds()
	m := image.NewRGBA(b)

	draw.Draw(m, b, src, image.Point{}, draw.Src)

	targetW := b.Dx() / 5
	targetH := b.Dy() / 5

	resizedLogo := s.resizeImage(logo, targetW, targetH)

	offset := image.Pt(
		(b.Dx()-targetW)/2,
		(b.Dy()-targetH)/2,
	)

	draw.Draw(m, image.Rect(offset.X, offset.Y, offset.X+targetW, offset.Y+targetH), resizedLogo, image.Point{}, draw.Over)
	return m
}
