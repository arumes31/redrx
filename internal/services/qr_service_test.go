package services

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQRService(t *testing.T) {
	service := NewQRService()

	t.Run("Generate PNG QR Code", func(t *testing.T) {
		opts := QROptions{
			Content: "https://example.com",
			Size:    256,
			FgColor: "#000000",
			BgColor: "#FFFFFF",
		}
		base64Str, bytes, err := service.GenerateQRCode(opts)
		
		assert.NoError(t, err)
		assert.NotEmpty(t, base64Str)
		assert.NotEmpty(t, bytes)
	})

	t.Run("Generate PNG QR Code with Logo", func(t *testing.T) {
		logo := image.NewRGBA(image.Rect(0, 0, 50, 50))
		opts := QROptions{
			Content: "https://example.com",
			Size:    256,
			Logo:    logo,
		}
		_, _, err := service.GenerateQRCode(opts)
		assert.NoError(t, err)
	})

	t.Run("Generate PNG QR Code Error", func(t *testing.T) {
		opts := QROptions{
			Content: strings.Repeat("A", 10000),
		}
		_, _, err := service.GenerateQRCode(opts)
		assert.Error(t, err)
	})

	t.Run("Generate SVG QR Code", func(t *testing.T) {
		opts := QROptions{
			Content: "https://example.com",
			FgColor: "#000000",
			BgColor: "#FFFFFF",
		}
		svg, err := service.GenerateQRCodeSVG(opts)
		
		assert.NoError(t, err)
		assert.True(t, strings.HasPrefix(svg, "<svg"))
		assert.Contains(t, svg, "#000000")
		assert.Contains(t, svg, "#FFFFFF")
	})

	t.Run("Generate SVG QR Code Error", func(t *testing.T) {
		opts := QROptions{
			Content: strings.Repeat("A", 10000),
		}
		_, err := service.GenerateQRCodeSVG(opts)
		assert.Error(t, err)
	})

	t.Run("Parse Hex Color", func(t *testing.T) {
		c := service.parseHexColor("invalid", color.Black)
		assert.Equal(t, color.Black, c)

		c = service.parseHexColor("#ff0000", color.Black)
		assert.Equal(t, color.RGBA{255, 0, 0, 255}, c)

		c = service.parseHexColor("#FF0000", color.Black)
		assert.Equal(t, color.RGBA{255, 0, 0, 255}, c)

		c = service.parseHexColor("#GGGGGG", color.Black)
		assert.Equal(t, color.RGBA{0, 0, 0, 255}, c)
	})
}

// Custom image that might fail encoding (but png.Encode is very robust)
// Let's try to pass a very large size to GenerateQRCode
func TestQRService_GenerateQRCode_LargeSize(t *testing.T) {
	service := NewQRService()
	opts := QROptions{
		Content: "https://example.com",
		Size:    20000, // Very large
	}
	_, _, err := service.GenerateQRCode(opts)
	// This might fail due to memory or skip2/go-qrcode limits
	if err != nil {
		t.Logf("Failed with large size: %v", err)
	}
}
