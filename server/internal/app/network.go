package app

import (
	"b0k3ts/internal/pkg/auth"
	"log/slog"

	"github.com/gin-gonic/gin"
)

func (app *App) Serve() {

	// Create a Gin router
	r := gin.Default()

	oAuth := auth.New(app.Config.OIDC, app.BadgerDB)

	v1 := r.Group("/api/v1")
	{
		v1.GET("/healthz", HealthzCheck)

		oidc := v1.Group("/oidc")
		{
			oidc.GET("/login", oAuth.Login)
			oidc.GET("/callback", oAuth.Callback)
			oidc.POST("/authenticate", oAuth.Authorize)
		}

	}

	slog.Info("listening on " + app.Config.Host + ":" + app.Config.Port)

	// Run the server
	//
	err := r.Run(app.Config.Host + ":" + app.Config.Port)
	if err != nil {
		slog.Error("failed to run gin router: ", err)
		return
	}

	return

}
