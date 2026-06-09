package apollo

import (
	"context"
	stdjson "encoding/json"
	"strings"

	"github.com/apolloconfig/agollo/v4"
	apolloconfig "github.com/apolloconfig/agollo/v4/env/config"

	baseConfig "github.com/tx7do/go-wind-plugins/config"
)

var (
	_ baseConfig.Reader       = (*apollo)(nil)
	_ baseConfig.ValueWatcher = (*apollo)(nil)
)

type apollo struct {
	client agollo.Client
	opt    *options
}

func NewSource(opts ...Option) *apollo {
	op := options{}
	for _, o := range opts {
		o(&op)
	}
	client, err := agollo.StartWithConfig(func() (*apolloconfig.AppConfig, error) {
		return &apolloconfig.AppConfig{
			AppID:            op.appid,
			Cluster:          op.cluster,
			NamespaceName:    op.namespace,
			IP:               op.endpoint,
			IsBackupConfig:   op.isBackupConfig,
			Secret:           op.secret,
			BackupConfigPath: op.backupPath,
		}, nil
	})
	if err != nil {
		panic(err)
	}
	return &apollo{client: client, opt: &op}
}

// resolveNamespace returns the namespace to use. If key is empty the first
// configured namespace is used.
func (e *apollo) resolveNamespace(key string) string {
	if key != "" {
		return key
	}
	namespaces := strings.Split(e.opt.namespace, ",")
	if len(namespaces) > 0 {
		return namespaces[0]
	}
	return e.opt.namespace
}

func (e *apollo) getConfig(ns string) ([]byte, error) {
	next := map[string]any{}
	e.client.GetConfigCache(ns).Range(func(key, value any) bool {
		resolve(genKey(ns, key.(string)), value, next)
		return true
	})
	return stdjson.Marshal(next)
}

func (e *apollo) getOriginConfig(ns string) ([]byte, error) {
	value, err := e.client.GetConfigCache(ns).Get("content")
	if err != nil {
		return nil, err
	}
	return []byte(value.(string)), nil
}

// Load implements [baseConfig.Reader].
// It returns the raw configuration bytes for the given namespace (or the
// configured default namespace when key is empty).
func (e *apollo) Load(_ context.Context, key string) ([]byte, error) {
	ns := e.resolveNamespace(key)

	if e.opt.originConfig && strings.Contains(ns, ".") &&
		!strings.HasSuffix(ns, "."+properties) &&
		(format(ns) == yaml || format(ns) == yml || format(ns) == json) {
		return e.getOriginConfig(ns)
	}
	return e.getConfig(ns)
}

// WatchValue implements [baseConfig.ValueWatcher].
// It returns a channel that receives the new value each time the configuration
// for the given namespace (or the configured default) changes.
func (e *apollo) WatchValue(ctx context.Context, key string) (<-chan []byte, error) {
	ns := e.resolveNamespace(key)
	return newWatchValueChannel(ctx, e, ns)
}
