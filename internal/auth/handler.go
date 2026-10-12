package auth

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Hhh22121/go/internal/apperr"
	"github.com/Hhh22121/go/internal/httpx"
	"github.com/Hhh22121/go/internal/user"
)

var errInvalidCredentials = fmt.Errorf("%w: invalid credentials", apperr.ErrUnauthorized)

// dummyHash is compared against when the email is unknown so that login
// takes roughly the same time whether or not the account exists.
var dummyHash, _ = HashPassword("dummy-password-for-timing")

type handler struct {
	users  *user.Service
	tokens *TokenService
}

func RegisterRoutes(rg *gin.RouterGroup, users *user.Service, tokens *TokenService) {
	h := &handler{users: users, tokens: tokens}
	g := rg.Group("/auth")
	g.POST("/register", h.register)
	g.POST("/login", h.login)
}

type registerRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8,max=72"`
}

func (h *handler) register(c *gin.Context) {
	var req registerRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	hash, err := HashPassword(req.Password)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	u, err := h.users.Create(c.Request.Context(), req.Email, hash)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"user": u})
}

type loginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required,max=72"`
}

func (h *handler) login(c *gin.Context) {
	var req loginRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	u, err := h.users.GetByEmail(c.Request.Context(), req.Email)
	if errors.Is(err, apperr.ErrNotFound) {
		CheckPassword(dummyHash, req.Password)
		httpx.WriteError(c, errInvalidCredentials)
		return
	}
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	if !CheckPassword(u.PasswordHash, req.Password) {
		httpx.WriteError(c, errInvalidCredentials)
		return
	}
	tok, err := h.tokens.Issue(u.ID)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"access_token": tok,
		"token_type":   "Bearer",
		"expires_in":   int(h.tokens.TTL().Seconds()),
	})
}
