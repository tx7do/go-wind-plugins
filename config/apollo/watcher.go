package apollo

import (
	"context"
	stdjson "encoding/json"
	"log"
	"strings"

	"github.com/apolloconfig/agollo/v4/storage"
)

// valueChangeListener receives apollo change events and pushes the new value
// (as raw bytes) to the output channel.
type valueChangeListener struct {
	out       chan<- []byte
	apollo    *apollo
	namespace string
}

func (c *valueChangeListener) onChange(namespace string, changes map[string]*storage.ConfigChange) []byte {
	// For structured namespaces (yaml/yml/json with a "content" key), return
	// the raw content directly.
	if strings.Contains(namespace, ".") && !strings.HasSuffix(namespace, "."+properties) &&
		(format(namespace) == yaml || format(namespace) == yml || format(namespace) == json) {
		if value, ok := changes["content"]; ok {
			return []byte(value.NewValue.(string))
		}
	}

	// Otherwise, resolve flat key-value pairs into a nested map and marshal to
	// JSON.
	next := make(map[string]any)
	for key, change := range changes {
		resolve(genKey(namespace, key), change.NewValue, next)
	}
	val, err := stdjson.Marshal(next)
	if err != nil {
		log.Printf("apollo could not handle namespace %s: %v", namespace, err)
		return nil
	}
	return val
}

func (c *valueChangeListener) OnChange(changeEvent *storage.ChangeEvent) {
	if changeEvent.Namespace != c.namespace {
		return
	}
	data := c.onChange(changeEvent.Namespace, changeEvent.Changes)
	if data == nil {
		return
	}
	select {
	case c.out <- data:
	case <-context.Background().Done():
	}
}

func (c *valueChangeListener) OnNewestChange(_ *storage.FullChangeEvent) {}

// newWatchValueChannel creates a channel that receives the new value each time
// the configuration for the given namespace changes. The channel is closed
// when ctx is cancelled.
func newWatchValueChannel(ctx context.Context, a *apollo, namespace string) (<-chan []byte, error) {
	out := make(chan []byte, 1)
	listener := &valueChangeListener{
		out:       out,
		apollo:    a,
		namespace: namespace,
	}
	a.client.AddChangeListener(listener)

	go func() {
		defer close(out)
		a.client.RemoveChangeListener(listener)
		<-ctx.Done()
	}()

	return out, nil
}
