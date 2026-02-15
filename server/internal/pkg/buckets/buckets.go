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

const OctetStream = "application/octet-stream"
const BucketIdPrefix = "bucket-"

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
		slog.Error("presign download failed. failed to bind json", "err", err)
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
	q.Set("response-content-type", OctetStream)

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
	req, ok := bindMoveRequest(c)
	if !ok {
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
	if isSingleObjectMove(req) {
		if err := moveSingleObject(ctx, mio, bucketConfig.BucketName, req); err != nil {
			respondMoveError(c, err)
			return
		}
		c.JSON(200, ObjectMoveResponse{Moved: 1})
		return
	}

	moved, err := moveByPrefix(ctx, mio, bucketConfig.BucketName, req)
	if err != nil {
		respondMoveError(c, err)
		return
	}
	c.JSON(200, ObjectMoveResponse{Moved: moved})
}

func bindMoveRequest(c *gin.Context) (ObjectMoveRequest, bool) {
	var req ObjectMoveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("move failed. failed to bind json", "err", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return ObjectMoveRequest{}, false
	}
	return req, true
}

func isSingleObjectMove(req ObjectMoveRequest) bool {
	return req.FromKey != "" || req.ToKey != ""
}

func moveSingleObject(ctx context.Context, mio *minio.Client, bucketName string, req ObjectMoveRequest) error {
	if req.FromKey == "" || req.ToKey == "" {
		return httpError{status: 400, msg: "from_key and to_key are required for single object move"}
	}
	if req.FromKey == req.ToKey {
		return httpError{status: 400, msg: "from_key and to_key must be different"}
	}

	if err := ensureDestinationAbsent(ctx, mio, bucketName, req.ToKey, req.Overwrite); err != nil {
		return err
	}

	return copyAndDelete(ctx, mio, bucketName, req.FromKey, req.ToKey,
		"failed to copy object",
		"failed to delete source object after copy",
	)
}

func moveByPrefix(ctx context.Context, mio *minio.Client, bucketName string, req ObjectMoveRequest) (int, error) {
	fromPrefix, toPrefix, err := validatePrefixMove(req)
	if err != nil {
		return 0, err
	}

	ch := mio.ListObjects(ctx, bucketName, minio.ListObjectsOptions{
		Prefix:    fromPrefix,
		Recursive: true,
	})

	moved := 0
	for obj := range ch {
		if err := obj.Err; err != nil {
			slog.Error("failed to list objects for move", "err", err)
			return moved, httpError{status: 400, msg: err.Error()}
		}

		n, err := moveOnePrefixObject(ctx, mio, bucketName, req, fromPrefix, toPrefix, obj.Key)
		if err != nil {
			return moved, err
		}
		moved += n
	}

	return moved, nil
}

func validatePrefixMove(req ObjectMoveRequest) (fromPrefix, toPrefix string, err error) {
	fromPrefix = normalizePrefix(req.FromPrefix)
	toPrefix = normalizePrefix(req.ToPrefix)

	if fromPrefix == "" || toPrefix == "" {
		return "", "", httpError{status: 400, msg: "either (from_key,to_key) or (from_prefix,to_prefix) must be provided"}
	}
	if fromPrefix == toPrefix {
		return "", "", httpError{status: 400, msg: "from_prefix and to_prefix must be different"}
	}
	return fromPrefix, toPrefix, nil
}

func moveOnePrefixObject(
	ctx context.Context,
	mio *minio.Client,
	bucketName string,
	req ObjectMoveRequest,
	fromPrefix, toPrefix, fromKey string,
) (int, error) {
	rel := strings.TrimPrefix(fromKey, fromPrefix)
	if rel == "" {
		// Shouldn't happen for real objects, but keep it safe.
		return 0, nil
	}

	newKey := toPrefix + rel

	if err := ensureDestinationAbsent(ctx, mio, bucketName, newKey, req.Overwrite); err != nil {
		return 0, prefixMoveConflictOrErr(err, newKey)
	}

	if err := copyAndDelete(ctx, mio, bucketName, fromKey, newKey,
		"failed to copy object during prefix move",
		"failed to delete source object during prefix move",
	); err != nil {
		return 0, withMoveKeys(err, fromKey, newKey)
	}

	return 1, nil
}

func prefixMoveConflictOrErr(err error, newKey string) error {
	he, ok := err.(httpError)
	if !ok || he.status != 409 {
		return err
	}
	return httpError{
		status: 409,
		body: gin.H{
			"error":   "destination object already exists",
			"object":  newKey,
			"message": "set overwrite=true to replace existing objects",
		},
	}
}

func withMoveKeys(err error, fromKey, toKey string) error {
	he, ok := err.(httpError)
	if !ok || he.status != 400 {
		return err
	}
	// Preserve earlier behavior: include from/to for easier debugging.
	if he.body == nil {
		he.body = gin.H{"error": he.msg, "from": fromKey, "to": toKey}
	}
	return he
}

func ensureDestinationAbsent(ctx context.Context, mio *minio.Client, bucketName, key string, overwrite bool) error {
	if overwrite {
		return nil
	}
	_, err := mio.StatObject(ctx, bucketName, key, minio.StatObjectOptions{})
	if err == nil {
		return httpError{status: 409, msg: "destination object already exists"}
	}
	return nil
}

func copyAndDelete(
	ctx context.Context,
	mio *minio.Client,
	bucketName, fromKey, toKey string,
	copyLogMsg, deleteLogMsg string,
) error {
	src := minio.CopySrcOptions{Bucket: bucketName, Object: fromKey}
	dst := minio.CopyDestOptions{Bucket: bucketName, Object: toKey}

	if _, err := mio.CopyObject(ctx, dst, src); err != nil {
		slog.Error(copyLogMsg, "err", err)
		return httpError{status: 400, msg: err.Error()}
	}

	if err := mio.RemoveObject(ctx, bucketName, fromKey, minio.RemoveObjectOptions{}); err != nil {
		slog.Error(deleteLogMsg, "err", err)
		return httpError{status: 400, msg: err.Error()}
	}

	return nil
}

type httpError struct {
	status int
	msg    string
	body   gin.H
}

func (e httpError) Error() string { return e.msg }

func respondMoveError(c *gin.Context, err error) {
	if he, ok := err.(httpError); ok {
		if he.body != nil {
			c.JSON(he.status, he.body)
			return
		}
		c.JSON(he.status, gin.H{"error": he.msg})
		return
	}
	c.JSON(400, gin.H{"error": err.Error()})
}

func (app *App) MultipartInitiate(c *gin.Context) {

	ctx := context.Background()

	var req MultipartInitiateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("multipart initiate failed. failed to bind json", "err", err)
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
		contentType = OctetStream
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
		slog.Error("multipart presign part failed. failed to bind json", "err", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Key == "" || req.UploadID == "" {
		c.JSON(400, gin.H{"error": "multipart presign failed. key and upload_id are required"})
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
		slog.Error("multipart complete failed. failed to bind json", "err", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Key == "" || req.UploadID == "" {
		c.JSON(400, gin.H{"error": "multipart complete failed. key and upload_id are required"})
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
		slog.Error("multipart abort failed. failed to bind json", "err", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Key == "" || req.UploadID == "" {
		c.JSON(400, gin.H{"error": "multipart abort failed. key and upload_id are required"})
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
	userInfo, ok := tokenUserOrRespond(c)
	if !ok {
		return
	}

	req, ok := bindDeleteConnectionRequest(c)
	if !ok {
		return
	}

	bucketConfig, ok := getBucketConfigOrRespond(c, app.DB, req.BucketId)
	if !ok {
		return
	}

	if !isAuthorizedForBucket(*app, userInfo, bucketConfig) {
		// Preserve previous behavior: silently succeed even if not authorized.
		c.JSON(200, gin.H{"message": "Bucket connection deleted successfully"})
		return
	}

	if err := badgerDB.DeleteKV(app.DB, BucketIdPrefix+req.BucketId); err != nil {
		slog.Error(err.Error())
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": "Bucket connection deleted successfully"})
}

func tokenUserOrRespond(c *gin.Context) (auth.User, bool) {
	userInfo, _ := auth.TokenToUserData(c.GetHeader("Authorization"))
	if strings.TrimSpace(userInfo.Email) == "" {
		slog.Error("token user or response failed. failed to get token id")
		c.JSON(400, gin.H{"error": "token user or response failed. failed to get token id"})
		return auth.User{}, false
	}
	return userInfo, true
}

func bindDeleteConnectionRequest(c *gin.Context) (BucketDeleteRequest, bool) {
	var req BucketDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("delete connection failed. failed to bind json: ", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return BucketDeleteRequest{}, false
	}
	return req, true
}

func getBucketConfigOrRespond(c *gin.Context, db *badger.DB, bucketID string) (BucketConfig, bool) {
	res, err := badgerDB.PullKV(db, BucketIdPrefix+bucketID)
	if err != nil {
		slog.Error(err.Error())
		c.JSON(400, gin.H{"error": err.Error()})
		return BucketConfig{}, false
	}

	var bucketConfig BucketConfig
	if err := json.Unmarshal(res, &bucketConfig); err != nil {
		slog.Error(err.Error())
		c.JSON(400, gin.H{"error": err.Error()})
		return BucketConfig{}, false
	}

	return bucketConfig, true
}

func isAuthorizedForBucket(app App, userInfo auth.User, bucketConfig BucketConfig) bool {
	if userInfo.Administrator {
		return true
	}

	// Direct user allowlist
	for _, u := range bucketConfig.AuthorizedUsers {
		if u == userInfo.Email {
			return true
		}
	}

	// Group allowlist (including admin group)
	for _, g := range userInfo.Groups {
		if g == app.OIDCConfig.AdminGroup {
			return true
		}
		for _, allowed := range bucketConfig.AuthorizedGroups {
			if g == allowed {
				return true
			}
		}
	}

	return false
}

func (app *App) ListConnection(c *gin.Context) {
	userInfo, ok := tokenUserOrRespond(c)
	if !ok {
		return
	}

	configs, ok := listBucketConfigsOrRespond(c, app.DB)
	if !ok {
		return
	}

	c.JSON(200, filterAuthorizedBucketConfigs(*app, userInfo, configs))
}

func listBucketConfigsOrRespond(c *gin.Context, db *badger.DB) ([]BucketConfig, bool) {
	raw := scanByPrefix(db, "bucket-")

	cfgs := make([]BucketConfig, 0, len(raw))
	for _, val := range raw {
		cfg, err := unmarshalBucketConfig(val)
		if err != nil {
			slog.Error(err.Error())
			c.JSON(400, gin.H{"error": err.Error()})
			return nil, false
		}
		cfgs = append(cfgs, cfg)
	}
	return cfgs, true
}

func unmarshalBucketConfig(b []byte) (BucketConfig, error) {
	var cfg BucketConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return BucketConfig{}, err
	}
	return cfg, nil
}

func filterAuthorizedBucketConfigs(app App, userInfo auth.User, cfgs []BucketConfig) []BucketConfig {
	out := make([]BucketConfig, 0, len(cfgs))
	for _, cfg := range cfgs {
		if isAuthorizedForBucket(app, userInfo, cfg) {
			out = append(out, cfg)
		}
	}
	return out
}

func (app *App) AddConnection(c *gin.Context) {

	// Getting User ID from JWT Token
	//
	userInfo, _ := auth.TokenToUserData(c.GetHeader("Authorization"))

	if userInfo.Email == "" {
		slog.Error("add connection failed. failed to get token id")
		c.JSON(400, gin.H{"error": "add connection failed. failed to get token id"})
		return
	}

	// Obtaining New Bucket Config From User
	//
	var bucketConfig BucketConfig
	if err := c.ShouldBindJSON(&bucketConfig); err != nil {
		slog.Error("add connection failed. failed to bind json: ", err)
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
	err = badgerDB.PutKV(app.DB, BucketIdPrefix+bucketConfig.BucketName, res)
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
		contentType = OctetStream
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
		slog.Error("download failed. failed to bind json: ", err)
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
	c.Header("Content-Type", OctetStream)
	c.Header("Content-Length", fmt.Sprintf("%d", stats.Size))

	c.Data(http.StatusOK, OctetStream, data)

	return
}

func (app *App) Delete(c *gin.Context) {

	var req ObjectDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("delete failed. failed to bind json: ", err)
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
	userInfo, ok := tokenUserOrRespond(c)
	if !ok {
		return nil
	}

	name, ok := bucketNameOrRespond(c, bucketName)
	if !ok {
		return nil
	}

	bucketConfig, ok := getBucketConfigOrRespond(c, app.DB, name)
	if !ok {
		return nil
	}

	if !isAuthorizedForBucket(app, userInfo, bucketConfig) {
		c.JSON(400, gin.H{"error": "Unauthorized"})
		return nil
	}

	return &bucketConfig
}

func bucketNameOrRespond(c *gin.Context, bucketName string) (string, bool) {
	if strings.TrimSpace(bucketName) != "" {
		return bucketName, true
	}

	var req ObjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("authorize failed. failed to bind json: ", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return "", false
	}

	name := strings.TrimSpace(req.BucketName)
	if name == "" {
		c.JSON(400, gin.H{"error": "bucket is required"})
		return "", false
	}

	return name, true
}
