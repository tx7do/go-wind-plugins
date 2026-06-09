package etcd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	baseConfig "github.com/tx7do/go-wind-plugins/config"
)

var (
	_ baseConfig.Reader       = (*source)(nil)
	_ baseConfig.ValueWatcher = (*source)(nil)
)

type source struct {
	client  *clientv3.Client
	options *options
}

func New(client *clientv3.Client, opts ...Option) (*source, error) {
	o := &options{
		ctx:    context.Background(),
		path:   "",
		prefix: false,
	}

	for _, opt := range opts {
		opt(o)
	}

	if o.path == "" {
		return nil, errors.New("path invalid")
	}

	return &source{
		client:  client,
		options: o,
	}, nil
}

// resolveKey returns the key to use for the given caller-provided key.
// If key is empty the configured default path is used.
func (s *source) resolveKey(key string) string {
	if key != "" {
		return key
	}
	return s.options.path
}

// Load implements [baseConfig.Reader].
// It returns the raw value stored under key (or the configured path when key
// is empty).
func (s *source) Load(ctx context.Context, key string) ([]byte, error) {
	path := s.resolveKey(key)

	var opts []clientv3.OpOption
	if s.options.prefix {
		opts = append(opts, clientv3.WithPrefix())
	}

	rsp, err := s.client.Get(ctx, path, opts...)
	if err != nil {
		return nil, wrapConnError("get key", path, err)
	}
	if len(rsp.Kvs) == 0 {
		return nil, nil
	}
	return rsp.Kvs[0].Value, nil
}

// WatchValue implements [baseConfig.ValueWatcher].
// It returns a channel that receives the new value each time the data under
// key (or the configured path) changes. The channel is closed when ctx is
// cancelled or the etcd watch stream ends.
func (s *source) WatchValue(ctx context.Context, key string) (<-chan []byte, error) {
	path := s.resolveKey(key)

	// short-timeout probe to check etcd reachability
	probeCtx, probeCancel := context.WithTimeout(ctx, 2*time.Second)
	defer probeCancel()
	if _, err := s.client.Get(probeCtx, path, clientv3.WithLimit(1)); err != nil {
		return nil, wrapConnError("create watcher", path, err)
	}

	watchCtx, cancel := context.WithCancel(ctx)
	var opts []clientv3.OpOption
	if s.options.prefix {
		opts = append(opts, clientv3.WithPrefix())
	}
	etcdCh := s.client.Watch(watchCtx, path, opts...)

	out := make(chan []byte, 1)
	go func() {
		defer close(out)
		defer cancel()
		for resp := range etcdCh {
			if err := resp.Err(); err != nil {
				return
			}
			for _, ev := range resp.Events {
				if ev.Type == clientv3.EventTypePut {
					select {
					case out <- ev.Kv.Value:
					case <-watchCtx.Done():
						return
					}
				}
			}
		}
	}()
	return out, nil
}

// ---- error helpers (unchanged) ----

// isConnError reports whether err looks like a connection/network problem.
func isConnError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	e := strings.ToLower(err.Error())
	indicators := []string{
		"connection refused", "connection reset", "no available endpoints",
		"transport is closing", "i/o timeout", "timeout", "connection timed out",
		"tls:", "connection reset by peer", "eof",
	}
	for _, sub := range indicators {
		if strings.Contains(e, sub) {
			return true
		}
	}
	return false
}

// wrapConnError adds a human-friendly prefix for connection-related errors.
func wrapConnError(op string, path string, err error) error {
	if err == nil {
		return nil
	}
	if isConnError(err) {
		if path == "" {
			return fmt.Errorf("etcd: %s failed (cannot reach etcd server): %w", op, err)
		}
		return fmt.Errorf("etcd: %s failed for path %s (cannot reach etcd server): %w", op, path, err)
	}
	return err
}
