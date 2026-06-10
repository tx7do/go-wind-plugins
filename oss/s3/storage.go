package s3

import (
	"context"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/tx7do/go-wind/log"
)

type Storage struct {
	client *awss3.Client
	bucket string
}

func NewStorage(cfg *Config) *Storage {
	if cfg == nil {
		log.GetLogger().Error(context.Background(), "missing s3 configuration")
		return nil
	}

	return &Storage{
		client: NewClient(cfg),
		bucket: cfg.Bucket,
	}
}

func (s *Storage) SDK() *awss3.Client {
	if s == nil {
		return nil
	}

	return s.client
}

func (s *Storage) Bucket() string {
	if s == nil {
		return ""
	}

	return s.bucket
}

func (s *Storage) PutObject(ctx context.Context, key string, body io.Reader, contentType string) (*awss3.PutObjectOutput, error) {
	if s == nil || s.client == nil {
		return nil, ErrNilClient
	}
	if s.bucket == "" {
		return nil, ErrEmptyBucket
	}
	if key == "" {
		return nil, ErrEmptyObjectKey
	}
	if isNilReader(body) {
		return nil, ErrNilObjectBody
	}

	preparedBody, size, err := prepareBody(body)
	if err != nil {
		return nil, err
	}

	input := &awss3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          preparedBody,
		ContentLength: aws.Int64(size),
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}

	return s.client.PutObject(ctx, input)
}

func (s *Storage) GetObject(ctx context.Context, key string) (*awss3.GetObjectOutput, error) {
	if s == nil || s.client == nil {
		return nil, ErrNilClient
	}
	if s.bucket == "" {
		return nil, ErrEmptyBucket
	}
	if key == "" {
		return nil, ErrEmptyObjectKey
	}

	return s.client.GetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
}
