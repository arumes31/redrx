package handlers

import (
	"encoding/base64"
	"image"
	"mime/multipart"
	"net/http"

	_ "image/jpeg"
	_ "image/png"

	"redrx/internal/services"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

type IndexForm struct {
	LongURL          string                `form:"long_url" binding:"required,url"`
	CustomCode       string                `form:"custom_code"`
	ExpiryHours      *int                  `form:"expiry_hours"`
	Password         string                `form:"password"`
	IOSTargetURL     string                `form:"ios_target_url"`
	AndroidTargetURL string                `form:"android_target_url"`
	QRFgColor        string                `form:"qr_fg_color"`
	QRBgColor        string                `form:"qr_bg_color"`
	LogoFile         *multipart.FileHeader `form:"logo_file"`

	// Security
	IsLocked         bool   `form:"is_locked"`
	AllowedIPs       string `form:"allowed_ips"`
	SplashPage       bool   `form:"splash_page"`
	SensitiveWarning bool   `form:"sensitive_warning"`
}

func (h *Handler) ShowIndex(c *gin.Context) {
	session := sessions.Default(c)
	user := session.Get("user_id")

	c.HTML(http.StatusOK, "index.html", gin.H{
		"User": user,
	})
}

func (h *Handler) HandleShortenForm(c *gin.Context) {
	var form IndexForm
	if err := c.ShouldBind(&form); err != nil {
		c.HTML(http.StatusOK, "index.html", gin.H{
			"Error": "Invalid Input: " + err.Error(),
		})
		return
	}

	session := sessions.Default(c)
	userIDVal := session.Get("user_id")
	var userID *uint
	if userIDVal != nil {
		uid := userIDVal.(uint)
		userID = &uid
	}

	// Process Logo File
	var logoImg image.Image
	if form.LogoFile != nil {
		file, err := form.LogoFile.Open()
		if err == nil {
			defer file.Close()
			img, _, err := image.Decode(file)
			if err == nil {
				logoImg = img
			}
		}
	}

	dto := services.ShortenDTO{
		UserID:           userID,
		LongURL:          form.LongURL,
		CustomCode:       form.CustomCode,
		ExpiryHours:      form.ExpiryHours,
		Password:         form.Password,
		IOSTargetURL:     form.IOSTargetURL,
		AndroidTargetURL: form.AndroidTargetURL,
		IPAddress:        c.ClientIP(),

		IsLocked:         form.IsLocked,
		AllowedIPs:       form.AllowedIPs,
		SplashPage:       form.SplashPage,
		SensitiveWarning: form.SensitiveWarning,
	}

	newURL, err := h.shortenerService.CreateShortURL(dto)
	if err != nil {
		c.HTML(http.StatusOK, "index.html", gin.H{
			"Error": "Failed to shorten: " + err.Error(),
		})
		return
	}

	shortURL := "https://" + c.Request.Host + "/" + newURL.ShortCode

	// Generate QR Code with Custom Colors
	qrOpts := services.QROptions{
		Content: shortURL,
		Size:    256,
		FgColor: form.QRFgColor,
		BgColor: form.QRBgColor,
		Logo:    logoImg,
	}

	// Default Logic if empty (handled in service, but good to be explicit if needed)
	if qrOpts.FgColor == "" {
		qrOpts.FgColor = "#000000"
	}
	if qrOpts.BgColor == "" {
		qrOpts.BgColor = "#FFFFFF"
	}

	qrData, _, _ := h.qrService.GenerateQRCode(qrOpts)

	// Generate SVG
	svgContent, _ := h.qrService.GenerateQRCodeSVG(qrOpts)
	svgBase64 := base64.StdEncoding.EncodeToString([]byte(svgContent))

	c.HTML(http.StatusOK, "index.html", gin.H{
		"Message":   "URL Shortened Successfully!",
		"ShortURL":  shortURL,
		"ShortCode": newURL.ShortCode,
		"QRData":    qrData,
		"QRSVG":     svgBase64,
		"User":      userIDVal,
	})
}
