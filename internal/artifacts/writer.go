package artifacts

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Writer interface {
	Write(ctx context.Context, key string, payload []byte) error
	Read(ctx context.Context, key string) ([]byte, error)
}

type GarageWriter struct {
	client *minio.Client
	bucket string
}

func NewGarageWriter(endpoint, accessKey, secretKey, bucket string) (*GarageWriter, error) {
	host := endpoint
	secure := false
	if parsed, err := url.Parse(endpoint); err == nil && parsed.Host != "" {
		host = parsed.Host
		secure = parsed.Scheme == "https"
	}
	host = strings.TrimRight(host, "/")
	client, err := minio.New(host, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: secure,
		Region: "garage",
	})
	if err != nil {
		return nil, err
	}
	return &GarageWriter{client: client, bucket: bucket}, nil
}

func (w *GarageWriter) Write(ctx context.Context, key string, payload []byte) error {
	_, err := w.client.PutObject(ctx, w.bucket, key, bytes.NewReader(payload), int64(len(payload)), minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})
	return err
}

func (w *GarageWriter) Read(ctx context.Context, key string) ([]byte, error) {
	obj, err := w.client.GetObject(ctx, w.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	return io.ReadAll(obj)
}
