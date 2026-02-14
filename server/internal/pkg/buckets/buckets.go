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
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/cors"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/samber/lo"
)

type PresignDownloadRequest struct {
	Bucket         string `json:"bucket"`                    // bucket connection id
	Key            string `json:"key"`                       // object key/path in the bucket
	ExpiresSeconds int64  `json:"expires_seconds,omitempty"` // optional; default 900
	Disposition    string `json:"disposition,omitempty"`     // optional: "attachment" (default) or "inline"
	Filename       string `json:"filename,omitempty"`        // optional: override filename in Content-Disposition
}

type PresignDownloadResponse struct {
	URL string `json:"url"`
}

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

type ObjectMoveRequest struct {
	Bucket     string `json:"bucket"`
	FromKey    string `json:"from_key,omitempty"`    // move a single object
	ToKey      string `json:"to_key,omitempty"`      // move a single object
	FromPrefix string `json:"from_prefix,omitempty"` // move a "folder" (prefix)
	ToPrefix   string `json:"to_prefix,omitempty"`   // move a "folder" (prefix)
	Overwrite  bool   `json:"overwrite,omitempty"`   // optional; default false
}

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
type ObjectMoveResponse struct {
	Moved int `json:"moved"`
}

func normalizePrefix(p string) string {
	if p == "" {
		return ""
	}
	// Treat prefixes as "folders" => ensure trailing slash for predictable trimming.
	if !strings.HasSuffix(p, "/") {
		return p + "/"
	}
	return p
}

func (app *App) PresignDownload(c *gin.Context) {
	var req PresignDownloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("failed to bind json", "err", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Bucket == "" || req.Key == "" {
		c.JSON(400, gin.H{"error": "bucket and key are required"})
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

	// Build response header overrides for browser-friendly downloads.
	disp := strings.ToLower(strings.TrimSpace(req.Disposition))
	if disp != "inline" {
		disp = "attachment"
	}

	filename := strings.TrimSpace(req.Filename)
	if filename == "" {
		filename = path.Base(req.Key)
		if filename == "." || filename == "/" || filename == "" {
			filename = "download"
		}
	}

	q := make(url.Values, 2)
	q.Set("response-content-disposition", fmt.Sprintf("%s; filename=%q", disp, filename))
	q.Set("response-content-type", "application/octet-stream")

	ctx := context.Background()
	u, err := mio.Presign(ctx, http.MethodGet, bucketConfig.BucketName, req.Key, time.Duration(expires)*time.Second, q)
	if err != nil {
		slog.Error("failed to presign download url", "err", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, PresignDownloadResponse{URL: u.String()})
}
func (app *App) Move(c *gin.Context) {
	var req ObjectMoveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("failed to bind json", "err", err)
		c.JSON(400, gin.H{"error": err.Error()})
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

	ctx := context.Background()

	// Determine mode: single-object move vs prefix move.
	if req.FromKey != "" || req.ToKey != "" {
		if req.FromKey == "" || req.ToKey == "" {
			c.JSON(400, gin.H{"error": "from_key and to_key are required for single object move"})
			return
		}
		if req.FromKey == req.ToKey {
			c.JSON(400, gin.H{"error": "from_key and to_key must be different"})
			return
		}

		// If not overwriting, check destination doesn't exist.
		if !req.Overwrite {
			_, err := mio.StatObject(ctx, bucketConfig.BucketName, req.ToKey, minio.StatObjectOptions{})
			if err == nil {
				c.JSON(409, gin.H{"error": "destination object already exists"})
				return
			}
		}

		src := minio.CopySrcOptions{
			Bucket: bucketConfig.BucketName,
			Object: req.FromKey,
		}
		dst := minio.CopyDestOptions{
			Bucket: bucketConfig.BucketName,
			Object: req.ToKey,
		}

		if _, err := mio.CopyObject(ctx, dst, src); err != nil {
			slog.Error("failed to copy object", "err", err)
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}

		if err := mio.RemoveObject(ctx, bucketConfig.BucketName, req.FromKey, minio.RemoveObjectOptions{}); err != nil {
			slog.Error("failed to delete source object after copy", "err", err)
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}

		c.JSON(200, ObjectMoveResponse{Moved: 1})
		return
	}

	fromPrefix := normalizePrefix(req.FromPrefix)
	toPrefix := normalizePrefix(req.ToPrefix)

	if fromPrefix == "" || toPrefix == "" {
		c.JSON(400, gin.H{"error": "either (from_key,to_key) or (from_prefix,to_prefix) must be provided"})
		return
	}
	if fromPrefix == toPrefix {
		c.JSON(400, gin.H{"error": "from_prefix and to_prefix must be different"})
		return
	}

	// Move all objects under fromPrefix to toPrefix (server-side copy+delete).
	moved := 0

	ch := mio.ListObjects(ctx, bucketConfig.BucketName, minio.ListObjectsOptions{
		Prefix:    fromPrefix,
		Recursive: true,
	})

	for obj := range ch {
		if obj.Err != nil {
			slog.Error("failed to list objects for move", "err", obj.Err)
			c.JSON(400, gin.H{"error": obj.Err.Error()})
			return
		}

		rel := strings.TrimPrefix(obj.Key, fromPrefix)
		if rel == "" {
			// Shouldn't happen for real objects, but keep it safe.
			continue
		}
		newKey := toPrefix + rel

		if !req.Overwrite {
			_, err := mio.StatObject(ctx, bucketConfig.BucketName, newKey, minio.StatObjectOptions{})
			if err == nil {
				c.JSON(409, gin.H{
					"error":   "destination object already exists",
					"object":  newKey,
					"message": "set overwrite=true to replace existing objects",
				})
				return
			}
		}

		src := minio.CopySrcOptions{
			Bucket: bucketConfig.BucketName,
			Object: obj.Key,
		}
		dst := minio.CopyDestOptions{
			Bucket: bucketConfig.BucketName,
			Object: newKey,
		}

		if _, err := mio.CopyObject(ctx, dst, src); err != nil {
			slog.Error("failed to copy object during prefix move", "from", obj.Key, "to", newKey, "err", err)
			c.JSON(400, gin.H{"error": err.Error(), "from": obj.Key, "to": newKey})
			return
		}

		if err := mio.RemoveObject(ctx, bucketConfig.BucketName, obj.Key, minio.RemoveObjectOptions{}); err != nil {
			slog.Error("failed to delete source object during prefix move", "key", obj.Key, "err", err)
			c.JSON(400, gin.H{"error": err.Error(), "key": obj.Key})
			return
		}

		moved++
	}

	c.JSON(200, ObjectMoveResponse{Moved: moved})
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
	Prefix     string `json:"prefix"`
	BucketName string `json:"bucket"`
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
	err = badgerDB.PutKV(app.DB, "bucket-"+bucketConfig.BucketName, res)
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

func (app *App) DownloadNative(c *gin.Context) {
	bucketID := c.Param("bucket")
	key := c.Param("key") // includes leading "/" because of the wildcard
	key = strings.TrimPrefix(key, "/")

	if bucketID == "" || key == "" {
		c.JSON(400, gin.H{"error": "bucket and key are required"})
		return
	}

	bucketConfig := authorizeAndExtract(*app, c, bucketID)
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

	obj, err := mio.GetObject(ctx, bucketConfig.BucketName, key, minio.GetObjectOptions{})
	if err != nil {
		slog.Error(err.Error())
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	defer obj.Close()

	st, err := obj.Stat()
	if err != nil {
		slog.Error(err.Error())
		c.JSON(404, gin.H{"error": "object not found"})
		return
	}

	// Pick a friendly filename for the browser.
	filename := path.Base(key)
	if filename == "." || filename == "/" || filename == "" {
		filename = "download"
	}

	disposition := strings.ToLower(strings.TrimSpace(c.Query("disposition")))
	if disposition != "inline" {
		disposition = "attachment"
	}

	contentType := st.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	c.Header("Content-Type", contentType)
	c.Header("Content-Length", fmt.Sprintf("%d", st.Size))
	c.Header("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, filename))

	// Stream the response (no buffering the whole object in memory).
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, obj); err != nil {
		// At this point headers/body may already be partially written; just log.
		slog.Error("stream download failed", "err", err, "bucket", bucketID, "key", key)
		return
	}
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

	err = mio.RemoveObject(ctx, bucketConfig.BucketName, req.Filename, minio.RemoveObjectOptions{})
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

		slog.Debug("REQ:", req)

		// Creating Bucket Instance Connection for User
		//
		res, err := badgerDB.PullKV(app.DB, "bucket-"+req.BucketName)
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
