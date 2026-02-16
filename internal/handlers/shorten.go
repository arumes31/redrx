package handlers

import (
	"net/http"

	"redrx/internal/services"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

type ShortenRequest struct {
	LongURL          string `json:"long_url" binding:"required,url"`
	CustomCode       string `json:"custom_code,omitempty"`
	ExpiryHours      *int   `json:"expiry_hours,omitempty"`
	Password         string `json:"password,omitempty"`
	IOSTargetURL     string `json:"ios_target_url,omitempty"`
	AndroidTargetURL string `json:"android_target_url,omitempty"`
}

// ShortenURL handles the API request to shorten a URL
func (h *Handler) ShortenURL(c *gin.Context) {
	var req ShortenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check Context (API Key Auth)
	var userID *uint
	if val, exists := c.Get("user_id"); exists {
		uid := val.(uint)
		userID = &uid
	} else {
		// Check Session
		session := sessions.Default(c)
		userIDVal := session.Get("user_id")
		if userIDVal != nil {
			uid := userIDVal.(uint)
			userID = &uid
		}
	}

	dto := services.ShortenDTO{
		UserID:           userID,
		LongURL:          req.LongURL,
		CustomCode:       req.CustomCode,
		ExpiryHours:      req.ExpiryHours,
		Password:         req.Password,
		IOSTargetURL:     req.IOSTargetURL,
		AndroidTargetURL: req.AndroidTargetURL,
		IPAddress:        c.ClientIP(),
	}

	newURL, err := h.shortenerService.CreateShortURL(dto)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"short_code": newURL.ShortCode,
		"short_url":  c.Request.Host + "/" + newURL.ShortCode,
	})
}
