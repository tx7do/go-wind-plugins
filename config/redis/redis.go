package redis

import (
	"context"
	"errors"
	"fmt"

	goredis "github.com/redis/go-redis/v9"

	baseConfig "github.com/tx7do/go-wind-plugins/config"
)

var (
	_ baseConfig.Reader       = (*source)(nil)
	_ baseConfig.ValueWatcher = (*source)(nil)
)

type source struct {
	client  goredis.UniversalClient
	options *options
}

// New creates a Redis-backed config source.
// The client and path options are required.
func New(client goredis.UniversalClient, opts ...Option) (*source, error) {
	if client == nil {
		return nil, errors.New("redis client is nil")
	}

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
// is empty). Returns nil, nil if the key does not exist.
func (s *source) Load(ctx context.Context, key string) ([]byte, error) {
	path := s.resolveKey(key)
	val, err := s.client.Get(ctx, path).Bytes()
	if err != nil {
		if err == goredis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("redis get %s: %w", path, err)
	}
	return val, nil
}

// WatchValue implements [baseConfig.ValueWatcher].
//
// It subscribes to a Redis Pub/Sub channel named "__windcfg__:<key>". When an
// external process updates the config value, it should publish to this channel
// (optionally with the new value as the message payload). If the message is
// non-empty, it is delivered directly; otherwise the watcher re-reads the key
// via GET.
//
// Convention for publishing a config update:
//
//	SET myapp:config '{"port":9090}'
//	PUBLISH __windcfg__:myapp:config ''
//
// The channel is closed when ctx is cancelled or the subscription ends.
func (s *source) WatchValue(ctx context.Context, key string) (<-chan []byte, error) {
	path := s.resolveKey(key)
	if path == "" {
		return nil, errors.New("no redis key specified")
	}
	channel := watchChannel(path)

	pubsub := s.client.Subscribe(ctx, channel)

	// Verify subscription before returning.
	_, err := pubsub.Receive(ctx)
	if err != nil {
		_ = pubsub.Close()
		return nil, fmt.Errorf("redis subscribe %s: %w", channel, err)
	}

	msgCh := pubsub.Channel()

	out := make(chan []byte, 1)
	go func() {
		defer close(out)
		defer func() { _ = pubsub.Close() }()

		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-msgCh:
				if !ok {
					return
				}
				if len(msg.Payload) > 0 {
					// The publisher included the new value in the message.
					select {
					case out <- []byte(msg.Payload):
					case <-ctx.Done():
						return
					}
				} else {
					// No payload — re-read from Redis.
					data, err := s.Load(ctx, path)
					if err != nil {
						continue
					}
					select {
					case out <- data:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return out, nil
}

// watchChannel returns the Pub/Sub channel name for the given config key.
func watchChannel(key string) string {
	return "__windcfg__:" + key
}
