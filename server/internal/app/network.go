package app

import (
	"b0k3ts/internal/pkg/auth"
	"b0k3ts/internal/pkg/buckets"
	"log/slog"

	"github.com/gin-gonic/gin"
)

func (app *App) Serve() {

	// Create a Gin router
	r := gin.Default()

	oAuth := auth.New(app.Config.OIDC, app.BadgerDB)
	bucket := buckets.NewConfig(app.BadgerDB)

	v1 := r.Group("/api/v1")
	{
		v1.GET("/healthz", HealthzCheck)

		oidc := v1.Group("/oidc")
		{
			oidc.GET("/login", oAuth.Login)
			oidc.GET("/callback", oAuth.Callback)
			oidc.POST("/authenticate", oAuth.Authorize)
		}

		bkt := v1.Group("/buckets")
		{
			bkt.POST("/add_connection", bucket.AddConnection)
			bkt.GET("/list_connections", bucket.ListConnection)
			bkt.POST("/delete_connection", bucket.DeleteConnection)
		}

		objects := v1.Group("/objects")
		{
			objects.POST("/upload", bucket.Upload)
			//objects.GET("/download", bucket.DownloadObject)
			//objects.POST("/delete", bucket.DeleteObject)
			objects.POST("/list", bucket.ListObjects)
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
