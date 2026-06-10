package oss

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/tx7do/go-wind/log"

	baseConfig "github.com/tx7do/go-wind-plugins/config"
)

var (
	_ baseConfig.Reader       = (*source)(nil)
	_ baseConfig.ValueWatcher = (*source)(nil)
)

const defaultPollInterval = 30 * time.Second

type source struct {
	client  *awss3.Client
	options *options
}

// New creates an S3-backed config source.
// The client and bucket options are required.
func New(client *awss3.Client, opts ...Option) (*source, error) {
	if client == nil {
		return nil, errors.New("s3 client is nil")
	}

	o := &options{
		ctx:          context.Background(),
		pollInterval: defaultPollInterval,
	}
	for _, opt := range opts {
		opt(o)
	}

	if o.bucket == "" {
		return nil, errors.New("bucket invalid")
	}

	return &source{
		client:  client,
		options: o,
	}, nil
}

// resolveKey returns the key to use for the given caller-provided key.
// If key is empty the configured default key is used.
func (s *source) resolveKey(key string) string {
	if key != "" {
		return key
	}
	return s.options.key
}

// Load implements [baseConfig.Reader].
// It downloads the object at key (or the configured default key when key is
// empty) from the configured S3 bucket and returns its raw contents.
func (s *source) Load(ctx context.Context, key string) ([]byte, error) {
	objKey := s.resolveKey(key)
	if objKey == "" {
		return nil, errors.New("no object key specified")
	}

	out, err := s.client.GetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(s.options.bucket),
		Key:    aws.String(objKey),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 get object %s/%s: %w", s.options.bucket, objKey, err)
	}
	defer out.Body.Close()

	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("s3 read body %s/%s: %w", s.options.bucket, objKey, err)
	}
	return data, nil
}

// WatchValue implements [baseConfig.ValueWatcher].
//
// S3-compatible object storage does not support push-mode notifications through
// the SDK, so this method uses polling: it periodically issues HeadObject and
// compares the ETag. When the ETag changes, it re-downloads the object and
// pushes the new value to the channel.
//
// The poll interval defaults to 30 seconds; override with WithPollInterval.
func (s *source) WatchValue(ctx context.Context, key string) (<-chan []byte, error) {
	objKey := s.resolveKey(key)
	if objKey == "" {
		return nil, errors.New("no object key specified")
	}

	out := make(chan []byte, 1)

	go func() {
		defer close(out)

		ticker := time.NewTicker(s.options.pollInterval)
		defer ticker.Stop()

		var lastETag string

		// Initial load — push current value and record ETag.
		head, err := s.headObject(ctx, objKey)
		if err != nil {
			log.GetLogger().Error(context.Background(), "[oss] initial head failed", "key", objKey, "error", err)
		} else {
			lastETag = head
			if data, err := s.Load(ctx, objKey); err == nil {
				select {
				case out <- data:
				case <-ctx.Done():
					return
				}
			} else {
				log.GetLogger().Error(context.Background(), "[oss] initial load failed", "key", objKey, "error", err)
			}
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				etag, err := s.headObject(ctx, objKey)
				if err != nil {
					log.GetLogger().Debug(context.Background(), "[oss] head failed during poll", "key", objKey, "error", err)
					continue
				}
				if etag == lastETag {
					continue
				}
				lastETag = etag
				data, err := s.Load(ctx, objKey)
				if err != nil {
					log.GetLogger().Error(context.Background(), "[oss] load after ETag change failed", "key", objKey, "error", err)
					continue
				}
				select {
				case out <- data:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}

// headObject returns the ETag of the object, or an error if it doesn't exist.
func (s *source) headObject(ctx context.Context, key string) (string, error) {
	out, err := s.client.HeadObject(ctx, &awss3.HeadObjectInput{
		Bucket: aws.String(s.options.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return "", err
	}
	if out.ETag != nil {
		return *out.ETag, nil
	}
	return "", nil
}
