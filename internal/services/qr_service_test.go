package services

import (
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
}
