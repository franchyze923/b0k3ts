package auth

import (
	"b0k3ts/configs"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"log/slog"

	"github.com/coreos/go-oidc"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"

	"golang.org/x/oauth2"
)

type Auth struct {
	config configs.OIDC
}

type JWTData struct {
	jwt.StandardClaims
	CustomClaims map[string]string `json:"custom_claims"`
}

type OIDCRegistrationUrl struct {
	RegistrationUrl string `json:"registrationUrl,omitempty"`
}

type Claims struct {
	Email         string `json:"email"`
	Name          string `json:"name"`
	PreferredName string `json:"preferred_username"`
	Groups        string `json:"groups"`
}

func New(config configs.OIDC) *Auth {
	return &Auth{
		config: config,
	}
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

	slog.Debug("%s", auth.config)

	ctx := oidc.ClientContext(context.Background(), client)
	provider, err := oidc.NewProvider(ctx, auth.config.ProviderUrl)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err})
		slog.Error(err.Error())
		return
	}

	oauth2Config := oauth2.Config{
		ClientID:     auth.config.ClientId,
		ClientSecret: auth.config.ClientSecret,
		RedirectURL:  auth.config.RedirectUrl,

		Endpoint: provider.Endpoint(),

		Scopes: []string{oidc.ScopeOpenID, "profile", "email", "groups"},
	}

	regUrl := oauth2Config.AuthCodeURL("code")

	var OIDCRegUrl = OIDCRegistrationUrl{
		RegistrationUrl: regUrl,
	}

	jm, err := json.Marshal(OIDCRegUrl)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err})
		slog.Error(err.Error())
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.JSON(http.StatusOK, jm)

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
	provider, err := oidc.NewProvider(ctx, auth.config.ProviderUrl)
	if err != nil {
		return
	}

	oauth2Config := oauth2.Config{
		ClientID:     auth.config.ClientId,
		ClientSecret: auth.config.ClientSecret,
		RedirectURL:  auth.config.RedirectUrl,

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
		ClientID: auth.config.ClientId,
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

	c.Redirect(http.StatusSeeOther, auth.config.PassRedirectUrl+rawAccessToken)
	return

}

func TokenToID(authToken, clientSecret string) string {

	authArr := strings.Split(authToken, " ")

	// JWT located at index 1
	//
	jwtToken := authArr[0]

	// We do not need to check for errors since we are already authorized
	//
	claims, _ := jwt.ParseWithClaims(jwtToken, &JWTData{},
		func(token *jwt.Token) (interface{}, error) {
			if jwt.SigningMethodHS256 != token.Method {
				return nil, errors.New("invalid signing algorithm")
			}

			return []byte(clientSecret), nil
		})

	data := claims.Claims.(*JWTData)
	userID := data.CustomClaims["userid"]

	return userID
}

// validateOIDC function used to validate users logged in using OIDC
func validateOIDC(authToken string, OIDCConfig configs.OIDC) error {
	rawAccessToken := authToken
	realmConfigURL := OIDCConfig.ProviderUrl

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
func Authorize(OIDCConfig configs.OIDC, w http.ResponseWriter, r *http.Request) error {

	slog.Info("Authorizing API Action")

	// Extracting request data
	//
	authToken := r.Header.Get("Authorization")

	err := validateOIDC(authToken, OIDCConfig)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return err
	}

	return nil
}
