package nacos

import (
	"context"

	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"

	baseConfig "github.com/tx7do/go-wind-plugins/config"
)

var (
	_ baseConfig.Reader       = (*Config)(nil)
	_ baseConfig.ValueWatcher = (*Config)(nil)
)

type Config struct {
	opts   options
	client config_client.IConfigClient
}

func New(client config_client.IConfigClient, opts ...Option) *Config {
	_options := options{}
	for _, o := range opts {
		o(&_options)
	}
	return &Config{client: client, opts: _options}
}

// resolveDataID returns the dataID to use. If key is non-empty it overrides
// the configured default.
func (c *Config) resolveDataID(key string) string {
	if key != "" {
		return key
	}
	return c.opts.dataID
}

// Load implements [baseConfig.Reader].
// It returns the raw configuration content for the given dataID (or the
// configured default dataID when key is empty).
func (c *Config) Load(ctx context.Context, key string) ([]byte, error) {
	dataID := c.resolveDataID(key)
	content, err := c.client.GetConfig(vo.ConfigParam{
		DataId: dataID,
		Group:  c.opts.group,
	})
	if err != nil {
		return nil, err
	}
	return []byte(content), nil
}

// WatchValue implements [baseConfig.ValueWatcher].
// It returns a channel that receives the new value each time the configuration
// for the given dataID (or the configured default) changes. The channel is
// closed when ctx is cancelled.
func (c *Config) WatchValue(ctx context.Context, key string) (<-chan []byte, error) {
	dataID := c.resolveDataID(key)

	out := make(chan []byte, 1)

	err := c.client.ListenConfig(vo.ConfigParam{
		DataId: dataID,
		Group:  c.opts.group,
		OnChange: func(_, group, dID, data string) {
			if dID == dataID && group == c.opts.group {
				select {
				case out <- []byte(data):
				case <-ctx.Done():
				}
			}
		},
	})
	if err != nil {
		return nil, err
	}

	go func() {
		defer close(out)
		<-ctx.Done()
		_ = c.client.CancelListenConfig(vo.ConfigParam{
			DataId: dataID,
			Group:  c.opts.group,
		})
	}()

	return out, nil
}
