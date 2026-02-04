package minio

import (
	"b0k3ts/configs"
	"b0k3ts/internal/pkg/auth"
	"b0k3ts/internal/pkg/badger"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/samber/lo"
	"go.yaml.in/yaml/v4"
)

type BucketConfig struct {
	BucketId        string `json:"bucket_id"`
	Endpoint        string `json:"endpoint"`
	AccessKeyId     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	Secure          bool   `json:"secure"`
	BucketName      string `json:"bucket_name"`
	Location        string `json:"location"`
}

type MinIO struct {
	client *minio.Client
	config BucketConfig
}

type Object struct {
	Key  string `json:"key"`
	Size int64  `json:"size"`
}

func AddConnection(c *gin.Context) {

	// Establish database connection
	//
	db := badger.InitializeDatabase()

	defer db.Close()

	// Getting server Config
	//
	val, err := badger.PullKV(db, "config")
	if err != nil {
		slog.Error(err.Error())
		c.JSON(400, gin.H{"error": err.Error()})
	}

	// Unmarshaling Config
	//
	var config configs.ServerConfig

	err = yaml.Unmarshal(val, &config)
	if err != nil {
		slog.Error(err.Error())
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Getting User ID from JWT Token
	//
	userID := auth.TokenToID(c.GetHeader("Authorization"), config.OIDC.ClientSecret)

	if userID == "" {
		slog.Error("failed to get token id")
		c.JSON(400, gin.H{"error": "failed to get token id"})
		return
	}

	// Obtaining New Bucket Config From User
	//
	var bucketConfig BucketConfig
	if err := c.ShouldBindJSON(&config); err != nil {
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
	err = badger.PutKV(db, userID+"-"+bucketConfig.BucketId, res)
	if err != nil {
		slog.Error(err.Error())
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": "Connection Added"})
}

func Connect(c *gin.Context) (*minio.Client, error) {

	var config BucketConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		slog.Error("failed to bind json: ", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return nil, err
	}

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
		c.JSON(400, gin.H{"error": err.Error()})
		return nil, err
	}

	return minioClient, nil

}

func (mio *MinIO) Upload(filename string, data []byte) error {

	ctx := context.Background()

	err := mio.client.MakeBucket(ctx, mio.config.BucketName, minio.MakeBucketOptions{
		Region: mio.config.Location})
	if err != nil {
		// Check to see if we already own this bucket (which happens if you run this twice)
		exists, errBucketExists := mio.client.BucketExists(ctx, mio.config.BucketName)
		if errBucketExists == nil && exists {
			slog.Info("Bucket %s already exists", mio.config.BucketName)
			return nil
		}

		slog.Error(err.Error())
		return err
	}

	slog.Info("Successfully created %s", mio.config.BucketName)

	// Upload File
	//

	info, err := mio.client.PutObject(ctx, mio.config.BucketName, filename, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: "application/octet-stream"})
	if err != nil {
		slog.Error(err.Error())
		return err
	}

	slog.Info("Successfully uploaded %s of size %d", filename, info.Size)

	return nil
}

func (mio *MinIO) Download(filename string) ([]byte, error) {

	ctx := context.Background()

	object, err := mio.client.GetObject(ctx, mio.config.BucketName, filename, minio.GetObjectOptions{})
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}

	defer object.Close()

	data, err := io.ReadAll(object)
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}

	slog.Info("Successfully downloaded %s", filename)
	return data, nil
}

func (mio *MinIO) Delete(filename string) error {

	ctx := context.Background()

	err := mio.client.RemoveObject(ctx, mio.config.BucketName, filename, minio.RemoveObjectOptions{})
	if err != nil {
		slog.Error(err.Error())
		return err
	}

	slog.Info("Successfully deleted %s", filename)
	return nil
}

func (mio *MinIO) ListBuckets() ([]string, error) {

	ctx := context.Background()

	buckets, err := mio.client.ListBuckets(ctx)
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}

	slog.Info("Successfully listed buckets")

	return lo.Map(buckets, func(u minio.BucketInfo, _ int) string { return u.Name }), nil
}

func (mio *MinIO) ListObjects(prefix string) ([]Object, error) {

	ctx := context.Background()

	channel := mio.client.ListObjects(ctx, mio.config.BucketName, minio.ListObjectsOptions{
		Recursive: true,
		Prefix:    "",
	})

	objects := make([]Object, 0)

	// Iterate through the channel
	//
	for object := range channel {

		if object.Err != nil {
			slog.Error(object.Err.Error())
			return nil, object.Err
		}

		objects = append(objects, Object{object.Key, object.Size})
	}

	slog.Info("Successfully listed objects")

	return objects, nil
}
