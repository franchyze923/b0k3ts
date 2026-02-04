package app

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func HealthzCheck(c *gin.Context) {
	c.Status(http.StatusOK)
}
