package minio

import (
	"bytes"
	"context"
	"io"
	"log"
	"log/slog"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/samber/lo"
)

type BucketConfig struct {
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

func New(config BucketConfig) *MinIO {

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
		log.Fatalln(err)
	}

	return &MinIO{
		client: minioClient,
		config: config,
	}
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
