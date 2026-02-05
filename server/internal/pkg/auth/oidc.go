package auth

import (
	"b0k3ts/configs"
	badgerDB "b0k3ts/internal/pkg/badger"
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"strings"
	"time"

	"log/slog"

	"github.com/coreos/go-oidc"
	"github.com/dgraph-io/badger/v4"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
	"go.yaml.in/yaml/v4"

	"golang.org/x/oauth2"
)

type Auth struct {
	Config   configs.OIDC
	BadgerDB *badger.DB
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

func New(config configs.OIDC, db *badger.DB) *Auth {
	return &Auth{
		Config:   config,
		BadgerDB: db,
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

	slog.Debug("%s", auth.Config)

	ctx := oidc.ClientContext(context.Background(), client)
	provider, err := oidc.NewProvider(ctx, auth.Config.ProviderUrl)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err})
		slog.Error(err.Error())
		return
	}

	oauth2Config := oauth2.Config{
		ClientID:     auth.Config.ClientId,
		ClientSecret: auth.Config.ClientSecret,
		RedirectURL:  auth.Config.RedirectUrl,

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
	provider, err := oidc.NewProvider(ctx, auth.Config.ProviderUrl)
	if err != nil {
		return
	}

	oauth2Config := oauth2.Config{
		ClientID:     auth.Config.ClientId,
		ClientSecret: auth.Config.ClientSecret,
		RedirectURL:  auth.Config.RedirectUrl,

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
		ClientID: auth.Config.ClientId,
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

	c.Redirect(http.StatusSeeOther, auth.Config.PassRedirectUrl+rawAccessToken)
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
func (auth *Auth) validateOIDC(authToken string) error {

	// Getting server Config
	//
	val, err := badgerDB.PullKV(auth.BadgerDB, "config")
	if err != nil {
		slog.Error(err.Error())
		return err
	}

	// Unmarshaling Config
	//
	var config configs.ServerConfig

	err = yaml.Unmarshal(val, &config)
	if err != nil {
		slog.Error(err.Error())
		return err
	}

	rawAccessToken := authToken
	realmConfigURL := config.OIDC.ProviderUrl

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

	slog.Debug("Authorization Token: %s", authToken)
	err := auth.validateOIDC(authToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"authenticated": true})
}
