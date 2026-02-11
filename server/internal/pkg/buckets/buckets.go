package buckets

import (
	"b0k3ts/configs"
	"b0k3ts/internal/pkg/auth"
	badgerDB "b0k3ts/internal/pkg/badger"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/cors"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/samber/lo"
)

type BucketConfig struct {
	BucketId         string   `json:"bucket_id"`
	Endpoint         string   `json:"endpoint"`
	AccessKeyId      string   `json:"access_key_id"`
	SecretAccessKey  string   `json:"secret_access_key"`
	Secure           bool     `json:"secure"`
	BucketName       string   `json:"bucket_name"`
	Location         string   `json:"location"`
	AuthorizedUsers  []string `json:"authorized_users"` // Email
	AuthorizedGroups []string `json:"authorized_groups"`
}

type BucketDeleteRequest struct {
	BucketId string `json:"bucket_id"`
}

// --- Direct Multipart Upload (server presigns, client uploads parts directly) ---

type MultipartInitiateRequest struct {
	Bucket      string `json:"bucket"`       // bucket connection id (same meaning as your other endpoints)
	Key         string `json:"key"`          // object key/path in the bucket
	ContentType string `json:"content_type"` // optional; defaults to application/octet-stream
}

type MultipartInitiateResponse struct {
	Bucket   string `json:"bucket"`
	Key      string `json:"key"`
	UploadID string `json:"upload_id"`
}

type MultipartPresignPartRequest struct {
	Bucket         string `json:"bucket"`
	Key            string `json:"key"`
	UploadID       string `json:"upload_id"`
	PartNumber     int    `json:"part_number"`     // 1..10000
	ExpiresSeconds int64  `json:"expires_seconds"` // optional; default 900
}

type MultipartPresignPartResponse struct {
	URL string `json:"url"`
}

type MultipartCompletedPart struct {
	PartNumber int    `json:"part_number"`
	ETag       string `json:"etag"`
}

type MultipartCompleteRequest struct {
	Bucket   string                   `json:"bucket"`
	Key      string                   `json:"key"`
	UploadID string                   `json:"upload_id"`
	Parts    []MultipartCompletedPart `json:"parts"`
}

type MultipartAbortRequest struct {
	Bucket   string `json:"bucket"`
	Key      string `json:"key"`
	UploadID string `json:"upload_id"`
}

func (app *App) MultipartInitiate(c *gin.Context) {

	ctx := context.Background()

	var req MultipartInitiateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("failed to bind json", "err", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Key == "" {
		c.JSON(400, gin.H{"error": "key is required"})
		return
	}

	bucketConfig := authorizeAndExtract(*app, c, req.Bucket)
	if bucketConfig == nil {
		return
	}

	core, err := ConnectCore(*bucketConfig)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	slog.Info("setting cors for bucket", "bucket", bucketConfig.BucketName)

	cfg := cors.Config{
		CORSRules: []cors.Rule{
			{
				AllowedOrigin: []string{"*"},
				AllowedMethod: []string{"PUT", "POST", "GET", "HEAD", "DELETE"},
				AllowedHeader: []string{"*"},
				ExposeHeader:  []string{"ETag"},
				MaxAgeSeconds: 3600,
			},
		},
	}

	if err := core.SetBucketCors(ctx, bucketConfig.BucketName, &cfg); err != nil {
		log.Fatal(err)
	}

	contentType := req.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	uploadID, err := core.NewMultipartUpload(ctx, bucketConfig.BucketName, req.Key, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		slog.Error("failed to initiate multipart upload", "err", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, MultipartInitiateResponse{
		Bucket:   req.Bucket,
		Key:      req.Key,
		UploadID: uploadID,
	})
}

func (app *App) MultipartPresignPart(c *gin.Context) {
	var req MultipartPresignPartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("failed to bind json", "err", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Key == "" || req.UploadID == "" {
		c.JSON(400, gin.H{"error": "key and upload_id are required"})
		return
	}
	if req.PartNumber < 1 || req.PartNumber > 10000 {
		c.JSON(400, gin.H{"error": "part_number must be between 1 and 10000"})
		return
	}

	bucketConfig := authorizeAndExtract(*app, c, req.Bucket)
	if bucketConfig == nil {
		return
	}

	mio, err := Connect(*bucketConfig)
	if err != nil {
		slog.Error(err.Error())
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	expires := req.ExpiresSeconds
	if expires <= 0 {
		expires = 900
	}
	if expires > 7*24*3600 {
		c.JSON(400, gin.H{"error": "expires_seconds too large"})
		return
	}

	ctx := context.Background()

	q := make(url.Values, 2)
	q.Set("partNumber", strconv.Itoa(req.PartNumber))
	q.Set("uploadId", req.UploadID)

	u, err := mio.Presign(ctx, http.MethodPut, bucketConfig.BucketName, req.Key, time.Duration(expires)*time.Second, q)
	if err != nil {
		slog.Error("failed to presign part url", "err", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, MultipartPresignPartResponse{URL: u.String()})
}

func (app *App) MultipartComplete(c *gin.Context) {
	var req MultipartCompleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("failed to bind json", "err", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Key == "" || req.UploadID == "" {
		c.JSON(400, gin.H{"error": "key and upload_id are required"})
		return
	}
	if len(req.Parts) == 0 {
		c.JSON(400, gin.H{"error": "parts is required"})
		return
	}

	bucketConfig := authorizeAndExtract(*app, c, req.Bucket)
	if bucketConfig == nil {
		return
	}

	core, err := ConnectCore(*bucketConfig)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	parts := make([]minio.CompletePart, 0, len(req.Parts))
	for _, p := range req.Parts {
		if p.PartNumber < 1 || p.PartNumber > 10000 || p.ETag == "" {
			c.JSON(400, gin.H{"error": "each part must have valid part_number and non-empty etag"})
			return
		}
		parts = append(parts, minio.CompletePart{
			PartNumber: p.PartNumber,
			ETag:       p.ETag,
		})
	}

	sort.Slice(parts, func(i, j int) bool { return parts[i].PartNumber < parts[j].PartNumber })

	ctx := context.Background()

	_, err = core.CompleteMultipartUpload(ctx, bucketConfig.BucketName, req.Key, req.UploadID, parts, minio.PutObjectOptions{})
	if err != nil {
		slog.Error("failed to complete multipart upload", "err", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": "Multipart upload completed"})
}

func (app *App) MultipartAbort(c *gin.Context) {
	var req MultipartAbortRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("failed to bind json", "err", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Key == "" || req.UploadID == "" {
		c.JSON(400, gin.H{"error": "key and upload_id are required"})
		return
	}

	bucketConfig := authorizeAndExtract(*app, c, req.Bucket)
	if bucketConfig == nil {
		return
	}

	core, err := ConnectCore(*bucketConfig)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	ctx := context.Background()

	err = core.AbortMultipartUpload(ctx, bucketConfig.BucketName, req.Key, req.UploadID)
	if err != nil {
		slog.Error("failed to abort multipart upload", "err", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": "Multipart upload aborted"})
}

// ... existing code ...

type ObjectRequest struct {
	Prefix   string `json:"prefix"`
	BucketId string `json:"bucket"`
}

type ObjectDownloadRequest struct {
	Bucket   string `json:"bucket"`
	Filename string `json:"filename"`
}
type ObjectDeleteRequest struct {
	Bucket   string `json:"bucket"`
	Filename string `json:"filename"`
}

type ObjectDownloadResponse struct {
	Content []byte `json:"content"`
}

type Buckets struct {
	Client   *minio.Client
	Config   BucketConfig
	BadgerDB *badger.DB
}

type App struct {
	DB         *badger.DB
	OIDCConfig configs.OIDC
}

type Object struct {
	Key         string `json:"key"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
}

func NewConfig(db *badger.DB, oidcConfig configs.OIDC) *App {
	return &App{DB: db, OIDCConfig: oidcConfig}
}

func scanByPrefix(db *badger.DB, prefixStr string) [][]byte {

	var results [][]byte

	err := db.View(func(txn *badger.Txn) error {
		// 1. Setup Iterator Options
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(prefixStr) // Optimization: skip irrelevant tables

		it := txn.NewIterator(opts)
		defer it.Close()

		// 2. Iterate
		// Seek: Jumps to the first key that matches or is greater than the prefix
		// ValidForPrefix: returns false when we pass the end of the prefix
		prefixBytes := []byte(prefixStr)
		for it.Seek(prefixBytes); it.ValidForPrefix(prefixBytes); it.Next() {
			item := it.Item()

			// 3. Retrieve Value
			err := item.Value(func(v []byte) error {
				valCopy, err := item.ValueCopy(nil)
				if err != nil {
					return err
				}

				results = append(results, valCopy)

				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		slog.Error(err.Error())
	}

	return results
}

func (app *App) DeleteConnection(c *gin.Context) {

	// Getting User ID from JWT Token
	//
	userInfo, _ := auth.TokenToUserData(c.GetHeader("Authorization"))

	if userInfo.Email == "" {
		slog.Error("failed to get token id")
		c.JSON(400, gin.H{"error": "failed to get token id"})
		return
	}

	// Obtaining New Bucket Config From User
	//
	var req BucketDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("failed to bind json: ", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Creating Bucket Instance Connection for User
	//
	res, err := badgerDB.PullKV(app.DB, "bucket-"+req.BucketId)
	if err != nil {
		slog.Error(err.Error())
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	var bucketConfig BucketConfig

	err = json.Unmarshal(res, &bucketConfig)
	if err != nil {
		slog.Error(err.Error())
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	authorized := false
	if userInfo.Administrator {
		authorized = true
	} else {
		for _, user := range bucketConfig.AuthorizedUsers {
			if user == userInfo.Email {
				authorized = true
				break
			}
		}

		for _, user := range userInfo.Groups {
			if user == app.OIDCConfig.AdminGroup {
				authorized = true
				break
			}
			for _, group := range bucketConfig.AuthorizedGroups {

				if user == group {
					authorized = true
					break
				}
			}
		}
	}

	if authorized {
		err := badgerDB.DeleteKV(app.DB, "bucket-"+req.BucketId)
		if err != nil {
			slog.Error(err.Error())
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}

	}

	c.JSON(200, gin.H{"message": "Bucket connection deleted successfully"})
}

func (app *App) ListConnection(c *gin.Context) {

	// Getting User ID from JWT Token
	//
	userInfo, _ := auth.TokenToUserData(c.GetHeader("Authorization"))

	if userInfo.Email == "" {
		slog.Error("failed to get token id")
		c.JSON(400, gin.H{"error": "failed to get token id"})
		return
	}

	var bucketConfig BucketConfig

	var bucketConfigs []BucketConfig

	resP := scanByPrefix(app.DB, "bucket-")

	for _, val := range resP {
		err := json.Unmarshal(val, &bucketConfig)
		if err != nil {
			slog.Error(err.Error())
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}

		authorized := false

		if userInfo.Administrator {
			authorized = true
		} else {

			for _, user := range bucketConfig.AuthorizedUsers {
				if user == userInfo.Email {

					authorized = true
					break
				}
			}

			for _, user := range userInfo.Groups {
				if user == app.OIDCConfig.AdminGroup {
					authorized = true
					break
				}
				for _, group := range bucketConfig.AuthorizedGroups {
					if user == group {
						authorized = true
						break
					}
				}
			}

		}

		if authorized {
			bucketConfigs = append(bucketConfigs, bucketConfig)

		}
	}

	c.JSON(200, bucketConfigs)
}

func (app *App) AddConnection(c *gin.Context) {

	// Getting User ID from JWT Token
	//
	userInfo, _ := auth.TokenToUserData(c.GetHeader("Authorization"))

	if userInfo.Email == "" {
		slog.Error("failed to get token id")
		c.JSON(400, gin.H{"error": "failed to get token id"})
		return
	}

	// Obtaining New Bucket Config From User
	//
	var bucketConfig BucketConfig
	if err := c.ShouldBindJSON(&bucketConfig); err != nil {
		slog.Error("failed to bind json: ", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Marshaling Bucket Config
	//
	res, err := json.Marshal(bucketConfig)
	if err != nil {
		slog.Error(err.Error())
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Creating Bucket Instance Connection for User
	//
	err = badgerDB.PutKV(app.DB, "bucket-"+bucketConfig.BucketId, res)
	if err != nil {
		slog.Error(err.Error())
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": "Connection Added"})
}

func Connect(config BucketConfig) (*minio.Client, error) {

	endpoint := config.Endpoint
	accessKeyID := config.AccessKeyId
	secretAccessKey := config.SecretAccessKey
	useSSL := config.Secure

	// Initialize minio client object.
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}

	return minioClient, nil
}

func ConnectCore(config BucketConfig) (*minio.Core, error) {
	endpoint := config.Endpoint
	accessKeyID := config.AccessKeyId
	secretAccessKey := config.SecretAccessKey
	useSSL := config.Secure

	core, err := minio.NewCore(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}
	return core, nil
}

func (app *App) Upload(c *gin.Context) {
	// Upload is now "Direct Multipart Upload" only: server no longer accepts file bytes.
	c.JSON(410, gin.H{
		"error":   "direct multipart upload required",
		"message": "use /api/v1/objects/multipart/initiate, /multipart/presign_part, /multipart/complete",
	})
	return
}

func (app *App) Download(c *gin.Context) {

	var req ObjectDownloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("failed to bind json: ", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	bucketConfig := authorizeAndExtract(*app, c, req.Bucket)
	if bucketConfig == nil {
		return
	}

	ctx := context.Background()

	mio, err := Connect(*bucketConfig)
	if err != nil {
		slog.Error(err.Error())
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	object, err := mio.GetObject(ctx, bucketConfig.BucketName, req.Filename, minio.GetObjectOptions{})
	if err != nil {
		slog.Error(err.Error())
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	defer object.Close()

	data, err := io.ReadAll(object)
	if err != nil {
		slog.Error(err.Error())
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	stats, err := object.Stat()
	if err != nil {
		slog.Error(err.Error())
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	slog.Info("Successfully downloaded %s", req.Filename)

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", req.Filename))
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Length", fmt.Sprintf("%d", stats.Size))

	c.Data(http.StatusOK, "application/octet-stream", data)

	return
}

func (app *App) Delete(c *gin.Context) {

	var req ObjectDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("failed to bind json: ", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	bucketConfig := authorizeAndExtract(*app, c, req.Bucket)
	if bucketConfig == nil {
		return
	}

	ctx := context.Background()

	mio, err := Connect(*bucketConfig)
	if err != nil {
		slog.Error(err.Error())
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	err = mio.RemoveObject(ctx, req.Bucket, req.Filename, minio.RemoveObjectOptions{})
	if err != nil {
		slog.Error(err.Error())
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	slog.Info("Successfully deleted %s", req.Filename)
	c.JSON(200, gin.H{"message": "Object deleted successfully"})
	return
}

func (mio *Buckets) ListBuckets() ([]string, error) {

	ctx := context.Background()

	buckets, err := mio.Client.ListBuckets(ctx)
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}

	slog.Info("Successfully listed buckets")

	return lo.Map(buckets, func(u minio.BucketInfo, _ int) string { return u.Name }), nil
}

func (app *App) ListObjects(c *gin.Context) {

	bucketConfig := authorizeAndExtract(*app, c, "")
	if bucketConfig == nil {
		return
	}

	mio, err := Connect(*bucketConfig)
	if err != nil {
		slog.Error(err.Error())
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	ctx := context.Background()

	channel := mio.ListObjects(ctx, bucketConfig.BucketName, minio.ListObjectsOptions{
		Recursive: true,
		Prefix:    "",
	})

	objects := make([]Object, 0)

	// Iterate through the channel
	//
	for object := range channel {

		if object.Err != nil {

			slog.Error(object.Err.Error())
			c.JSON(400, gin.H{"error": object.Err.Error()})
			return
		}

		objects = append(objects, Object{object.Key, object.Size, object.ContentType})
	}

	slog.Info("Successfully listed objects")

	c.JSON(200, objects)
}

func authorizeAndExtract(app App, c *gin.Context, bucketName string) *BucketConfig {

	// Getting User ID from JWT Token
	//
	userInfo, _ := auth.TokenToUserData(c.GetHeader("Authorization"))

	if userInfo.Email == "" {
		slog.Error("failed to get token id")
		c.JSON(400, gin.H{"error": "failed to get token id"})
		return nil
	}

	var bucketConfig BucketConfig

	if bucketName == "" {
		// Obtaining New Bucket Config From User
		//
		var req ObjectRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			slog.Error("failed to bind json: ", err)
			c.JSON(400, gin.H{"error": err.Error()})
			return nil
		}

		// Creating Bucket Instance Connection for User
		//
		res, err := badgerDB.PullKV(app.DB, "bucket-"+req.BucketId)
		if err != nil {
			slog.Error(err.Error())
			c.JSON(400, gin.H{"error": err.Error()})
			return nil
		}

		err = json.Unmarshal(res, &bucketConfig)
		if err != nil {
			slog.Error(err.Error())
			c.JSON(400, gin.H{"error": err.Error()})
			return nil
		}

	} else {
		res, err := badgerDB.PullKV(app.DB, "bucket-"+bucketName)
		if err != nil {
			slog.Error(err.Error())
			c.JSON(400, gin.H{"error": err.Error()})
			return nil
		}

		err = json.Unmarshal(res, &bucketConfig)
		if err != nil {
			slog.Error(err.Error())
			c.JSON(400, gin.H{"error": err.Error()})
			return nil
		}
	}

	authorized := false
	if userInfo.Administrator {
		authorized = true
	} else {
		for _, user := range bucketConfig.AuthorizedUsers {
			if user == userInfo.Email {
				authorized = true
				break
			}
		}

		for _, user := range userInfo.Groups {
			if user == app.OIDCConfig.AdminGroup {
				authorized = true
				break
			}
			for _, group := range bucketConfig.AuthorizedGroups {

				if user == group {
					authorized = true
					break
				}
			}
		}
	}
	if !authorized {
		c.JSON(400, gin.H{"error": "Unauthorized"})
		return nil

	}

	return &bucketConfig
}
