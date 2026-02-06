package auth

import (
	"net/http"
	"strings"
	"time"

	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
)

type LocalLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LocalLoginResponse struct {
	Authenticated bool   `json:"authenticated"`
	Token         string `json:"token,omitempty"`
	Username      string `json:"username,omitempty"`
}

type LocalClaims struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	jwt.StandardClaims
}

// LocalLogin validates local username/password and returns a signed JWT for the frontend.
// If you want the exact same behavior as OIDC (redirect with token), use LocalLoginRedirect below.
func (auth *Auth) LocalLogin(c *gin.Context) {
	var req LocalLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("failed to bind json: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	store := NewStore(auth.BadgerDB)
	rec, err := store.ValidateUser(req.Username, req.Password)
	if err != nil {
		// Keep this generic to avoid user enumeration.
		c.JSON(http.StatusUnauthorized, gin.H{"error": ErrInvalidUsernameOrPassword.Error()})
		return
	}

	// Token TTL (pick what you want)
	const tokenTTL = 24 * time.Hour

	now := time.Now().UTC()
	claims := LocalClaims{
		Username: rec.Username,
		Email:    rec.Email,
		StandardClaims: jwt.StandardClaims{
			Subject:   rec.Username,
			IssuedAt:  now.Unix(),
			ExpiresAt: now.Add(tokenTTL).Unix(),
		},
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	secret := strings.TrimSpace(auth.ServerConfig.JWTSecret)
	if secret == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server jwt secret is not configured"})
		return
	}

	signed, err := tok.SignedString([]byte(secret))
	if err != nil {
		slog.Error("failed to sign token: ", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sign token"})
		return
	}

	c.JSON(http.StatusOK, LocalLoginResponse{
		Authenticated: true,
		Token:         signed,
		Username:      rec.Username,
	})
}

// LocalLoginRedirect does the same as LocalLogin, but redirects the browser to your frontend,
// matching your OIDC "PassRedirectUrl + token" pattern.
func (auth *Auth) LocalLoginRedirect(c *gin.Context) {

	var req LocalLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("failed to bind json: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	store := NewStore(auth.BadgerDB)
	rec, err := store.ValidateUser(req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": ErrInvalidUsernameOrPassword.Error()})
		return
	}

	const tokenTTL = 24 * time.Hour
	now := time.Now().UTC()

	claims := LocalClaims{
		Username: rec.Username,
		Email:    rec.Email,
		StandardClaims: jwt.StandardClaims{
			Subject:   rec.Username,
			IssuedAt:  now.Unix(),
			ExpiresAt: now.Add(tokenTTL).Unix(),
		},
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	secret := strings.TrimSpace(auth.ServerConfig.JWTSecret)
	if secret == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server jwt secret is not configured"})
		return
	}

	signed, err := tok.SignedString([]byte(secret))
	if err != nil {
		slog.Error("failed to sign token: ", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sign token"})
		return
	}

	// Same frontend pattern as OIDC
	c.Redirect(http.StatusSeeOther, auth.OIDCConfig.PassRedirectUrl+signed)
}

// LocalAuthorize validates the local JWT from Authorization: Bearer <token>
func (auth *Auth) LocalAuthorize(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")

	if authHeader == "" {
		slog.Error("missing authorization header")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
		return
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header"})
		return
	}
	rawToken := strings.TrimSpace(parts[1])
	if rawToken == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	secret := strings.TrimSpace(auth.ServerConfig.JWTSecret)
	if secret == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server jwt secret is not configured"})
		return
	}

	claims := &LocalClaims{}
	parsed, err := jwt.ParseWithClaims(rawToken, claims, func(token *jwt.Token) (interface{}, error) {
		if jwt.SigningMethodHS256 != token.Method {
			return nil, ErrInvalidUsernameOrPassword
		}
		return []byte(secret), nil
	})
	if err != nil || !parsed.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"authenticated": true,
		"user_info": gin.H{
			"username": claims.Username,
		},
	})
}
