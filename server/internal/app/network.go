package app

import (
	"b0k3ts/configs"
	"b0k3ts/internal/pkg/auth"
	badgerDB "b0k3ts/internal/pkg/badger"
	"b0k3ts/internal/pkg/buckets"
	"b0k3ts/internal/pkg/kubernetes"
	"encoding/json"
	"log/slog"

	"github.com/gin-gonic/gin"
)

func (app *App) Serve() {

	// Create a Gin router
	r := gin.New()

	r.Use(gin.Recovery())
	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/api/v1/healthz"},
	}))

	if err := r.SetTrustedProxies(nil); err != nil {
		slog.Error("failed to set trusted proxies", "err", err)
		return
	}

	//
	var oic configs.OIDC

	res, err := badgerDB.PullKV(app.BadgerDB, "oidc-config")
	if err != nil {
		if err.Error() == "Key not found" {
			slog.Info("OIDC Not Configured")
		} else {
			slog.Error(err.Error())
			return
		}
		//return

	}

	if err == nil {
		err = json.Unmarshal(res, &oic)
		if err != nil {
			slog.Error(err.Error())
			return
		}
	}

	oAuth := auth.New(app.Config, oic, app.BadgerDB)
	bucket := buckets.NewConfig(app.BadgerDB, oic)
	localStore := auth.NewStore(app.BadgerDB)

	v1 := r.Group("/api/v1")
	{
		v1.GET("/healthz", HealthzCheck)

		oidc := v1.Group("/oidc")
		{
			oidc.GET("/login", oAuth.Login)
			oidc.GET("/callback", oAuth.Callback)
			oidc.POST("/authenticate", oAuth.Authorize)
			oidc.GET("/config", oAuth.GetConfig)
			oidc.POST("/configure", oAuth.Configure)
		}

		local := v1.Group("/local")
		{
			local.POST("/login", oAuth.LocalLogin)
			local.POST("/login_redirect", oAuth.LocalLoginRedirect)
			local.POST("/authenticate", oAuth.LocalAuthorize)

			// Local user management & self-service endpoints:
			RegisterLocalUserRoutes(local, localStore)
		}

		bkt := v1.Group("/buckets")
		{
			bkt.POST("/add_connection", bucket.AddConnection)
			bkt.GET("/list_connections", bucket.ListConnection)
			bkt.POST("/delete_connection", bucket.DeleteConnection)
		}

		objects := v1.Group("/objects")
		{
			// objects.POST("/upload", bucket.Upload) // removed: multipart only
			objects.POST("/download", bucket.Download)
			objects.POST("/delete", bucket.Delete)
			objects.POST("/list", bucket.ListObjects)

			// Direct Multipart Upload:
			objects.POST("/multipart/initiate", bucket.MultipartInitiate)
			objects.POST("/multipart/presign_part", bucket.MultipartPresignPart)
			objects.POST("/multipart/complete", bucket.MultipartComplete)
			objects.POST("/multipart/abort", bucket.MultipartAbort)
		}

		k8s := v1.Group("/kubernetes")
		{
			kubernetes.RegisterRoutes(k8s, app.BadgerDB)
		}

	}

	slog.Info("listening on " + app.Config.Host + ":" + app.Config.Port)

	// Run the server
	//
	err = r.Run(app.Config.Host + ":" + app.Config.Port)
	if err != nil {
		slog.Error("failed to run gin router: ", err)
		return
	}

	return

}
