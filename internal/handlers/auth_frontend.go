package handlers

import (
	"net/http"

	"redrx/internal/models"
	"redrx/internal/repository"
	"redrx/pkg/utils"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func ShowLogin(c *gin.Context) {
	c.HTML(http.StatusOK, "login.html", nil)
}

func ShowRegister(c *gin.Context) {
	c.HTML(http.StatusOK, "register.html", nil)
}

func HandleLoginForm(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")

	var user models.User
	result := repository.DB.Where("username = ? OR email = ?", username, username).First(&user)
	if result.Error != nil {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{"Error": "Invalid credentials"})
		return
	}

	if !utils.CheckPasswordHash(password, user.PasswordHash) {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{"Error": "Invalid credentials"})
		return
	}

	// Set Session
	session := sessions.Default(c)
	session.Set("user_id", user.ID)
	if err := session.Save(); err != nil {
		c.HTML(http.StatusInternalServerError, "login.html", gin.H{"Error": "Failed to save session"})
		return
	}

	c.Redirect(http.StatusFound, "/dashboard")
}

func HandleRegisterForm(c *gin.Context) {
	username := c.PostForm("username")
	email := c.PostForm("email")
	password := c.PostForm("password")

	// Check if user exists
	var existingUser models.User
	if err := repository.DB.Where("username = ? OR email = ?", username, email).First(&existingUser).Error; err == nil {
		c.HTML(http.StatusConflict, "register.html", gin.H{"Error": "Username or email already exists"})
		return
	}

	hashedPassword, err := utils.HashPassword(password)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "register.html", gin.H{"Error": "Failed to hash password"})
		return
	}

	newUser := models.User{
		Username:     username,
		Email:        email,
		PasswordHash: hashedPassword,
		APIKey:       utils.GenerateAPIKey(),
	}

	if err := repository.DB.Create(&newUser).Error; err != nil {
		c.HTML(http.StatusInternalServerError, "register.html", gin.H{"Error": "Failed to create user"})
		return
	}

	c.Redirect(http.StatusFound, "/login")
}
