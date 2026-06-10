package http

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tx7do/go-wind/log"

	baseConfig "github.com/tx7do/go-wind-plugins/config"
)

var (
	_ baseConfig.Reader       = (*source)(nil)
	_ baseConfig.ValueWatcher = (*source)(nil)
	_ baseConfig.Closer       = (*source)(nil)
)

const defaultPollInterval = 30 * time.Second

type source struct {
	options *options
	client  *http.Client
}

// New creates an HTTP-backed config source.
// The url option is required; an error is returned if it is empty.
func New(opts ...Option) (*source, error) {
	o := &options{
		ctx:          context.Background(),
		method:       http.MethodGet,
		pollInterval: defaultPollInterval,
	}
	for _, opt := range opts {
		opt(o)
	}

	if o.url == "" {
		return nil, errors.New("url invalid")
	}

	client := o.httpClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	return &source{
		options: o,
		client:  client,
	}, nil
}

// resolveKey returns the URL to fetch for the given caller-provided key.
// If key is empty the configured default URL is used.
func (s *source) resolveKey(key string) string {
	if key != "" {
		return key
	}
	return s.options.url
}

// buildRequest creates an *http.Request with configured headers and optional
// ETag/If-None-Match for conditional requests.
func (s *source) buildRequest(ctx context.Context, url, etag string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, s.options.method, url, nil)
	if err != nil {
		return nil, err
	}

	for k, v := range s.options.headers {
		req.Header.Set(k, v)
	}

	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	return req, nil
}

// Load implements [baseConfig.Reader].
// It performs an HTTP request to the configured URL (or key if provided) and
// returns the response body. HTTP 304 returns nil, nil (not modified).
func (s *source) Load(ctx context.Context, key string) ([]byte, error) {
	url := s.resolveKey(key)

	req, err := s.buildRequest(ctx, url, "")
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http %s: %s", url, resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	return data, nil
}

// WatchValue implements [baseConfig.ValueWatcher].
//
// HTTP does not support push notifications, so this method uses polling with
// conditional GET (ETag / If-None-Match). On each poll interval:
//  1. Sends a GET request with If-None-Match: <last-etag>
//  2. If the server returns 304, the value hasn't changed.
//  3. If the server returns 200, the new body and ETag are delivered.
//
// The initial fetch is performed immediately so the caller receives the
// current value without waiting for the first tick.
func (s *source) WatchValue(ctx context.Context, key string) (<-chan []byte, error) {
	url := s.resolveKey(key)

	out := make(chan []byte, 1)

	go func() {
		defer close(out)

		ticker := time.NewTicker(s.options.pollInterval)
		defer ticker.Stop()

		var lastETag string

		// Initial fetch.
		data, etag, err := s.fetchWithETag(ctx, url, "")
		if err != nil {
			log.GetLogger().Error(context.Background(), "[http] initial fetch failed", "url", url, "error", err)
		} else if data != nil {
			lastETag = etag
			select {
			case out <- data:
			case <-ctx.Done():
				return
			}
		} else if etag != "" {
			// 304 with no body — record ETag but don't push (nothing changed).
			lastETag = etag
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				data, etag, err := s.fetchWithETag(ctx, url, lastETag)
				if err != nil {
					log.GetLogger().Debug(context.Background(), "[http] poll fetch failed", "url", url, "error", err)
					continue
				}
				if etag != "" {
					lastETag = etag
				}
				if data == nil {
					// 304 Not Modified — nothing changed.
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

// fetchWithETag performs a conditional GET and returns the body, ETag, and error.
// If the response is 304, data is nil but etag may still be set.
func (s *source) fetchWithETag(ctx context.Context, url, etag string) (data []byte, respETag string, err error) {
	req, err := s.buildRequest(ctx, url, etag)
	if err != nil {
		return nil, "", err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	respETag = resp.Header.Get("ETag")

	if resp.StatusCode == http.StatusNotModified {
		return nil, respETag, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, respETag, fmt.Errorf("http %s: %s", url, resp.Status)
	}

	data, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, respETag, fmt.Errorf("read response body: %w", err)
	}

	return data, respETag, nil
}

// Close closes the underlying HTTP client's idle connections.
func (s *source) Close() error {
	if s.client != nil {
		s.client.CloseIdleConnections()
	}
	return nil
}
