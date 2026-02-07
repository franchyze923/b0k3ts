package auth

import (
	"b0k3ts/configs"
	badgerDB "b0k3ts/internal/pkg/badger"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"log/slog"

	"github.com/coreos/go-oidc"
	"github.com/dgraph-io/badger/v4"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"

	"golang.org/x/oauth2"
)

type Auth struct {
	ServerConfig configs.ServerConfig
	OIDCConfig   configs.OIDC
	BadgerDB     *badger.DB
}

type User struct {
	ID            string   `json:"id,omitempty"`
	Email         string   `json:"email,omitempty"`
	Name          string   `json:"name,omitempty"`
	PreferredName string   `json:"preferred_name,omitempty"`
	Groups        []string `json:"groups,omitempty"`
}

type JWTData struct {
	jwt.StandardClaims
	CustomClaims  map[string]string `json:"custom_claims"`
	Email         string            `json:"email"`
	PreferredName string            `json:"preferred_username"`
	Name          string            `json:"name"`
	Scope         string            `json:"scope"`
	EmailVerified bool              `json:"email_verified"`
	Groups        []string          `json:"groups"`
}

type OIDCRegistrationUrl struct {
	RegistrationUrl string `json:"registrationUrl,omitempty"`
}

type Claims struct {
	Email         string   `json:"email"`
	Name          string   `json:"name"`
	PreferredName string   `json:"preferred_username"`
	Groups        []string `json:"groups"`
}

func New(config configs.ServerConfig, oidcConfig configs.OIDC, db *badger.DB) *Auth {
	return &Auth{
		ServerConfig: config,
		OIDCConfig:   oidcConfig,
		BadgerDB:     db,
	}
}

func (auth *Auth) GetConfig(c *gin.Context) {

	ret, err := badgerDB.PullKV(auth.BadgerDB, "oidc-config")
	if err != nil {
		slog.Error(err.Error())
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.Data(200, "application/json", ret)

}

func (auth *Auth) Configure(c *gin.Context) {

	var req configs.OIDC
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("failed to bind json: ", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	ret, err := json.Marshal(req)
	if err != nil {
		slog.Error("failed to marshal json: ", err)
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	// Saving Config on Badger
	//
	err = badgerDB.PutKV(auth.BadgerDB, "oidc-config", ret)
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	c.JSON(200, gin.H{"message": "oidc config saved successfully"})

}

func (auth *Auth) Login(c *gin.Context) {

	slog.Info("Connecting to OIDC Provider")

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	client := &http.Client{
		Timeout:   time.Duration(24) * time.Hour,
		Transport: tr,
	}

	slog.Debug("%s", auth.OIDCConfig)

	ctx := oidc.ClientContext(context.Background(), client)
	provider, err := oidc.NewProvider(ctx, auth.OIDCConfig.ProviderUrl)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err})
		slog.Error(err.Error())
		return
	}

	oauth2Config := oauth2.Config{
		ClientID:     auth.OIDCConfig.ClientId,
		ClientSecret: auth.OIDCConfig.ClientSecret,
		RedirectURL:  auth.OIDCConfig.RedirectUrl,

		Endpoint: provider.Endpoint(),

		Scopes: []string{oidc.ScopeOpenID, "profile", "email", "groups"},
	}

	regUrl := oauth2Config.AuthCodeURL("code")

	var OIDCRegUrl = OIDCRegistrationUrl{
		RegistrationUrl: regUrl,
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.JSON(http.StatusOK, OIDCRegUrl)

	slog.Info("↳ Connected ✅ ")
	return

}

func (auth *Auth) Callback(c *gin.Context) {

	slog.Info("Callback")

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	client := &http.Client{
		Timeout:   time.Duration(24) * time.Hour,
		Transport: tr,
	}

	ctx := oidc.ClientContext(context.Background(), client)
	provider, err := oidc.NewProvider(ctx, auth.OIDCConfig.ProviderUrl)
	if err != nil {
		return
	}

	oauth2Config := oauth2.Config{
		ClientID:     auth.OIDCConfig.ClientId,
		ClientSecret: auth.OIDCConfig.ClientSecret,
		RedirectURL:  auth.OIDCConfig.RedirectUrl,

		Endpoint: provider.Endpoint(),

		Scopes: []string{oidc.ScopeOpenID, "profile", "email", "groups"},
	}

	oauth2Token, err := oauth2Config.Exchange(ctx, c.Query("code"))
	if err != nil {
		return
	}

	rawAccessToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return
	}

	oidcConfig := &oidc.Config{
		ClientID: auth.OIDCConfig.ClientId,
	}

	verifier := provider.Verifier(oidcConfig)

	id, err := verifier.Verify(ctx, rawAccessToken)
	if err != nil {
		return
	}

	var claims Claims

	if err := id.Claims(&claims); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err})
		slog.Error("Failed to retrieve claims: ", err)
		return
	}

	c.Redirect(http.StatusSeeOther, auth.OIDCConfig.PassRedirectUrl+rawAccessToken)
	return

}

// TokenToUserData extracts user info from either:
// - OIDC JWTs (id/access tokens with claims like email, preferred_username, name, groups)
// - Local JWTs (your HS256 tokens with claims like username/email)
//
// IMPORTANT: This function does NOT verify the signature.
// Use it only after you've already verified the token (OIDC: validateOIDC, Local: LocalAuthorize),
// or if you're okay with "best-effort display" and not authorization decisions.
func TokenToUserData(authToken string) (User, error) {

	raw := strings.TrimSpace(authToken)

	// Accept "Bearer <token>" or raw token
	if parts := strings.SplitN(raw, " ", 2); len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		raw = strings.TrimSpace(parts[1])
	}
	if raw == "" {
		return User{}, errors.New("empty token")
	}

	// Parse claims without verifying signature (we assume verification happened earlier).
	parser := new(jwt.Parser)
	claims := jwt.MapClaims{}
	_, _, err := parser.ParseUnverified(raw, claims)
	if err != nil {
		return User{}, err
	}

	getString := func(key string) string {
		if v, ok := claims[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}

	// Local tokens (your code): username/email
	localUsername := getString("username")
	localEmail := getString("email")

	// OIDC tokens: preferred_username/email/name
	oidcPreferred := getString("preferred_username")
	oidcEmail := getString("email")
	oidcName := getString("name")

	// Choose best available fields
	email := firstNonEmpty(localEmail, oidcEmail)
	preferred := firstNonEmpty(oidcPreferred, localUsername)
	name := firstNonEmpty(oidcName, preferred, email)

	groups := extractStringSlice(claims["groups"])

	// ID: prefer email, else preferred/username
	id := firstNonEmpty(email, preferred, localUsername, getString("sub"))

	return User{
		ID:            id,
		Email:         email,
		Name:          name,
		PreferredName: preferred,
		Groups:        groups,
	}, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func extractStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}

	// Common JWT decode type: []interface{}
	if arr, ok := v.([]interface{}); ok {
		out := make([]string, 0, len(arr))
		for _, item := range arr {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}

	// Sometimes it's already []string
	if arr, ok := v.([]string); ok {
		return arr
	}

	// Sometimes it's a single string
	if s, ok := v.(string); ok && s != "" {
		return []string{s}
	}

	return nil
}

// validateOIDC function used to validate users logged in using OIDC
func (auth *Auth) validateOIDC(authToken string) error {

	rawAccessToken := authToken
	realmConfigURL := auth.OIDCConfig.ProviderUrl

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{
		Timeout:   time.Duration(24) * time.Hour,
		Transport: tr,
	}

	ctx := oidc.ClientContext(context.Background(), client)
	provider, err := oidc.NewProvider(ctx, realmConfigURL)
	if err != nil {
		return err
	}

	oidcConfig := &oidc.Config{
		SkipClientIDCheck: true,
	}

	verifier := provider.Verifier(oidcConfig)
	_, err = verifier.Verify(ctx, rawAccessToken)
	if err != nil {
		return err
	}

	return nil
}

// Authorize function used to validate user JWT token expiration status
func (auth *Auth) Authorize(c *gin.Context) {

	slog.Info("Authorizing API Action")

	// Extracting request data
	//
	authToken := c.GetHeader("Authorization")

	err := auth.validateOIDC(authToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	userInfo, _ := TokenToUserData(authToken)
	c.JSON(http.StatusOK, gin.H{"authenticated": true, "user_info": userInfo})
}
