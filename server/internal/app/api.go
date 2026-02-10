package app

import (
	"b0k3ts/internal/pkg/auth"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func HealthzCheck(c *gin.Context) {
	c.Status(http.StatusOK)
}

type LocalUsersAPI struct {
	Store *auth.Store
}

func RegisterLocalUserRoutes(rg *gin.RouterGroup, store *auth.Store) {
	api := &LocalUsersAPI{Store: store}

	users := rg.Group("/users")
	{
		// Admin/user management
		users.POST("/exists", api.UserExists)              // admin-only (avoids user enumeration)
		users.POST("/ensure", api.EnsureUser)              // admin-only
		users.POST("/create", api.CreateUser)              // admin-only
		users.GET("/:username", api.GetUser)               // admin-only (returns safe fields)
		users.POST("/disable", api.DisableUser)            // admin-only
		users.POST("/delete", api.DeleteUser)              // admin-only
		users.POST("/update_password", api.UpdatePassword) // admin-only password reset

		// Auth / self-service
		users.POST("/validate", api.ValidateUser)          // public-ish (returns safe user info or error)
		users.POST("/change_password", api.ChangePassword) // self (or admin)
	}
}

func mustBeAdmin(c *gin.Context) bool {
	userInfo, _ := auth.TokenToUserData(c.GetHeader("Authorization"))
	if !userInfo.Administrator {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin required"})
		return false
	}
	return true
}

func isSelfOrAdmin(c *gin.Context, username string) bool {
	userInfo, _ := auth.TokenToUserData(c.GetHeader("Authorization"))
	if userInfo.Administrator {
		return true
	}

	u := strings.ToLower(strings.TrimSpace(username))
	// Best-effort matching for local users:
	// - prefer preferred username, else email
	if strings.ToLower(strings.TrimSpace(userInfo.PreferredName)) == u {
		return true
	}
	if strings.ToLower(strings.TrimSpace(userInfo.Email)) == u {
		return true
	}
	return false
}

type createUserRequest struct {
	Username      string `json:"username" binding:"required"`
	Password      string `json:"password" binding:"required"`
	Administrator bool   `json:"administrator"`
}

type ensureUserRequest struct {
	Username      string `json:"username" binding:"required"`
	Password      string `json:"password" binding:"required"`
	Administrator bool   `json:"administrator"`
}

type userExistsRequest struct {
	Username string `json:"username" binding:"required"`
}

type validateUserRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type changePasswordRequest struct {
	Username    string `json:"username" binding:"required"`
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

type updatePasswordRequest struct {
	Username    string `json:"username" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

type disableUserRequest struct {
	Username string `json:"username" binding:"required"`
	Disabled bool   `json:"disabled"`
}

type deleteUserRequest struct {
	Username string `json:"username" binding:"required"`
}

// userRecordSafe strips sensitive fields (PasswordHash).
type userRecordSafe struct {
	Username      string   `json:"username"`
	CreatedAt     any      `json:"created_at"`
	UpdatedAt     any      `json:"updated_at"`
	Disabled      bool     `json:"disabled"`
	Email         string   `json:"email"`
	Groups        []string `json:"groups"`
	Administrator bool     `json:"administrator"`
}

func toSafe(rec *auth.UserRecord) userRecordSafe {
	return userRecordSafe{
		Username:      rec.Username,
		CreatedAt:     rec.CreatedAt,
		UpdatedAt:     rec.UpdatedAt,
		Disabled:      rec.Disabled,
		Email:         rec.Email,
		Groups:        rec.Groups,
		Administrator: rec.Administrator,
	}
}

func (api *LocalUsersAPI) UserExists(c *gin.Context) {
	if !mustBeAdmin(c) {
		return
	}

	var req userExistsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	exists, err := api.Store.UserExists(req.Username)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"exists": exists})
}

func (api *LocalUsersAPI) EnsureUser(c *gin.Context) {
	if !mustBeAdmin(c) {
		return
	}

	var req ensureUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	created, err := api.Store.EnsureUser(req.Username, req.Password, req.Administrator)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"created": created})
}

func (api *LocalUsersAPI) CreateUser(c *gin.Context) {
	if !mustBeAdmin(c) {
		return
	}

	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := api.Store.CreateUser(req.Username, req.Password, req.Administrator)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, auth.ErrUserAlreadyExists) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user created"})
}

func (api *LocalUsersAPI) GetUser(c *gin.Context) {
	if !mustBeAdmin(c) {
		return
	}

	username := c.Param("username")
	rec, err := api.Store.GetUser(username)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, auth.ErrUserNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, toSafe(rec))
}

func (api *LocalUsersAPI) ValidateUser(c *gin.Context) {
	var req validateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rec, err := api.Store.ValidateUser(req.Username, req.Password)
	if err != nil {
		// return generic auth error
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"authenticated": true, "user": toSafe(rec)})
}

func (api *LocalUsersAPI) ChangePassword(c *gin.Context) {
	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !isSelfOrAdmin(c, req.Username) {
		c.JSON(http.StatusForbidden, gin.H{"error": "not allowed"})
		return
	}

	if err := api.Store.ChangePassword(req.Username, req.OldPassword, req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "password changed"})
}

func (api *LocalUsersAPI) UpdatePassword(c *gin.Context) {
	if !mustBeAdmin(c) {
		return
	}

	var req updatePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := api.Store.UpdatePassword(req.Username, req.NewPassword); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, auth.ErrUserNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "password updated"})
}

func (api *LocalUsersAPI) DisableUser(c *gin.Context) {
	if !mustBeAdmin(c) {
		return
	}

	var req disableUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if strings.TrimSpace(req.Username) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username is required"})
		return
	}

	if err := api.Store.DisableUser(req.Username, req.Disabled); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, auth.ErrUserNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user updated"})
}

func (api *LocalUsersAPI) DeleteUser(c *gin.Context) {
	if !mustBeAdmin(c) {
		return
	}

	var req deleteUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := api.Store.DeleteUser(req.Username); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user deleted"})
}
