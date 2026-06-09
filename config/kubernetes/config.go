package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	baseConfig "github.com/tx7do/go-wind-plugins/config"
)

var (
	_ baseConfig.Reader       = (*kube)(nil)
	_ baseConfig.ValueWatcher = (*kube)(nil)
)

type kube struct {
	opts   options
	client *kubernetes.Clientset
}

// New new a kubernetes config source.
func New(opts ...Option) *kube {
	op := options{}
	for _, o := range opts {
		o(&op)
	}
	return &kube{
		opts: op,
	}
}

func (k *kube) init() (err error) {
	var c *rest.Config
	if k.opts.KubeConfig != "" {
		if c, err = clientcmd.BuildConfigFromFlags(k.opts.Master, k.opts.KubeConfig); err != nil {
			return err
		}
	} else {
		if c, err = rest.InClusterConfig(); err != nil {
			return err
		}
	}
	if k.client, err = kubernetes.NewForConfig(c); err != nil {
		return err
	}
	return nil
}

// resolveKey parses a key of the form "namespace/name/dataKey" and returns the
// parts. If the key does not contain slashes, the configured namespace is used
// and the key is treated as the ConfigMap name (returning all data merged).
func (k *kube) resolveKey(key string) (namespace, name, dataKey string) {
	parts := strings.SplitN(key, "/", 3)
	switch len(parts) {
	case 3:
		return parts[0], parts[1], parts[2]
	case 2:
		return parts[0], parts[1], ""
	default:
		return k.opts.Namespace, key, ""
	}
}

// configMapData extracts the value for dataKey from a ConfigMap. If dataKey is
// empty, all data entries are merged into a single JSON object.
func (k *kube) configMapData(cm v1.ConfigMap, dataKey string) ([]byte, error) {
	if dataKey != "" {
		val, ok := cm.Data[dataKey]
		if !ok {
			return nil, fmt.Errorf("key %q not found in configmap %s/%s", dataKey, cm.Namespace, cm.Name)
		}
		return []byte(val), nil
	}

	// merge all data entries
	var parts []string
	for _, v := range cm.Data {
		parts = append(parts, v)
	}
	return []byte(strings.Join(parts, "\n")), nil
}

// Load implements [baseConfig.Reader].
// The key should be "namespace/name/dataKey" or "namespace/name" (returns all
// merged data) or just "name" (uses the configured namespace).
func (k *kube) Load(ctx context.Context, key string) ([]byte, error) {
	if k.client == nil {
		if err := k.init(); err != nil {
			return nil, err
		}
	}

	ns, name, dataKey := k.resolveKey(key)
	if ns == "" {
		return nil, errors.New("namespace not specified")
	}

	cm, err := k.client.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return k.configMapData(*cm, dataKey)
}

// WatchValue implements [baseConfig.ValueWatcher].
// It watches the ConfigMap identified by key for changes and delivers the new
// value on the returned channel.
func (k *kube) WatchValue(ctx context.Context, key string) (<-chan []byte, error) {
	if k.client == nil {
		if err := k.init(); err != nil {
			return nil, err
		}
	}

	ns, name, dataKey := k.resolveKey(key)
	if ns == "" {
		return nil, errors.New("namespace not specified")
	}

	w, err := k.client.CoreV1().ConfigMaps(ns).Watch(ctx, metav1.ListOptions{
		LabelSelector: k.opts.LabelSelector,
		FieldSelector: k.opts.FieldSelector,
	})
	if err != nil {
		return nil, err
	}

	out := make(chan []byte, 1)
	go func() {
		defer close(out)
		defer w.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-w.ResultChan():
				if !ok {
					return
				}
				if ev.Object == nil {
					return
				}
				cm, ok := ev.Object.(*v1.ConfigMap)
				if !ok {
					continue
				}
				if cm.Name != name {
					continue
				}
				if ev.Type == watch.Deleted {
					return
				}
				data, err := k.configMapData(*cm, dataKey)
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
	}()

	return out, nil
}
