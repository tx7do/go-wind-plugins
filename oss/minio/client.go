package minio

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/tx7do/go-wind/log"
)

var (
	ErrNilClient      = errors.New("minio client is nil")
	ErrEmptyBucket    = errors.New("minio bucket is empty")
	ErrEmptyObjectKey = errors.New("minio object key is empty")
	ErrNilObjectBody  = errors.New("minio object body is nil")
)

type Storage struct {
	client *minio.Client
}

func NewClient(cfg *Config) *minio.Client {
	if cfg == nil {
		log.GetLogger().Error(context.Background(), "missing minio configuration")
		return nil
	}

	impl, err := minio.New(cfg.Endpoint,
		&minio.Options{
			Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, cfg.Token),
			Secure: cfg.UseSsl,
		},
	)
	if err != nil {
		log.GetLogger().Error(context.Background(), "failed opening connection to minio", "error", err)
		return nil
	}

	return impl
}

func NewStorage(cfg *Config) *Storage {
	return &Storage{client: NewClient(cfg)}
}

func (s *Storage) SDK() *minio.Client {
	if s == nil {
		return nil
	}

	return s.client
}

func (s *Storage) PutObject(ctx context.Context, bucket, key string, body io.Reader, contentType string) (minio.UploadInfo, error) {
	if s == nil || s.client == nil {
		return minio.UploadInfo{}, ErrNilClient
	}
	if bucket == "" {
		return minio.UploadInfo{}, ErrEmptyBucket
	}
	if key == "" {
		return minio.UploadInfo{}, ErrEmptyObjectKey
	}
	if isNilReader(body) {
		return minio.UploadInfo{}, ErrNilObjectBody
	}

	preparedBody, size, err := prepareBody(body)
	if err != nil {
		return minio.UploadInfo{}, err
	}

	return s.client.PutObject(ctx, bucket, key, preparedBody, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
}

func (s *Storage) GetObject(ctx context.Context, bucket, key string) (*minio.Object, error) {
	if s == nil || s.client == nil {
		return nil, ErrNilClient
	}
	if bucket == "" {
		return nil, ErrEmptyBucket
	}
	if key == "" {
		return nil, ErrEmptyObjectKey
	}

	return s.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
}

func isNilReader(body io.Reader) bool {
	if body == nil {
		return true
	}

	v := reflect.ValueOf(body)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func prepareBody(body io.Reader) (io.Reader, int64, error) {
	if rs, ok := body.(io.ReadSeeker); ok {
		size, err := readerSize(rs)
		if err != nil {
			return nil, 0, err
		}
		return rs, size, nil
	}

	data, err := io.ReadAll(body)
	if err != nil {
		return nil, 0, err
	}

	return bytes.NewReader(data), int64(len(data)), nil
}

func readerSize(rs io.ReadSeeker) (int64, error) {
	current, err := rs.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}

	end, err := rs.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, err
	}

	_, err = rs.Seek(current, io.SeekStart)
	if err != nil {
		return 0, err
	}

	return end - current, nil
}
