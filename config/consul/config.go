package consul

import (
	"context"
	"errors"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/api/watch"

	baseConfig "github.com/tx7do/go-wind-plugins/config"
)

var (
	_ baseConfig.Reader       = (*source)(nil)
	_ baseConfig.ValueWatcher = (*source)(nil)
)

type source struct {
	client  *api.Client
	options *options
}

func New(client *api.Client, opts ...Option) (*source, error) {
	o := &options{
		ctx:  context.Background(),
		path: "",
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
	kv, _, err := s.client.KV().Get(path, nil)
	if err != nil {
		return nil, err
	}
	if kv == nil {
		return nil, nil
	}
	return kv.Value, nil
}

// WatchValue implements [baseConfig.ValueWatcher].
// It returns a channel that receives the new value each time the data under
// key (or the configured path) changes. The channel is closed when ctx is
// cancelled or the consul watch plan stops.
func (s *source) WatchValue(ctx context.Context, key string) (<-chan []byte, error) {
	path := s.resolveKey(key)

	wp, err := watch.Parse(map[string]interface{}{"type": "key", "key": path})
	if err != nil {
		return nil, err
	}

	out := make(chan []byte, 1)
	wp.Handler = func(_ uint64, data interface{}) {
		if data == nil {
			return
		}
		kvPair, ok := data.(*api.KVPair)
		if !ok || kvPair == nil {
			return
		}
		select {
		case out <- kvPair.Value:
		case <-ctx.Done():
		}
	}

	go func() {
		defer close(out)
		_ = wp.RunWithClientAndHclog(s.client, nil)
	}()

	go func() {
		<-ctx.Done()
		wp.Stop()
	}()

	return out, nil
}
