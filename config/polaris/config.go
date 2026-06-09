package polaris

import (
	"context"
	"errors"
	"log"

	"github.com/polarismesh/polaris-go"
	"github.com/polarismesh/polaris-go/pkg/model"

	baseConfig "github.com/tx7do/go-wind-plugins/config"
)

var (
	_ baseConfig.Reader       = (*source)(nil)
	_ baseConfig.ValueWatcher = (*source)(nil)
)

type source struct {
	client  polaris.ConfigAPI
	options *options
}

func New(client polaris.ConfigAPI, opts ...Option) (*source, error) {
	o := &options{
		namespace: "default",
		fileGroup: "",
		fileName:  "",
	}

	for _, opt := range opts {
		opt(o)
	}

	if o.fileGroup == "" {
		return nil, errors.New("fileGroup invalid")
	}

	if o.fileName == "" {
		return nil, errors.New("fileName invalid")
	}

	return &source{
		client:  client,
		options: o,
	}, nil
}

// resolveFileName returns the fileName to use. If key is non-empty it overrides
// the configured default.
func (s *source) resolveFileName(key string) string {
	if key != "" {
		return key
	}
	return s.options.fileName
}

// Load implements [baseConfig.Reader].
// It returns the raw configuration file content for the given fileName (or the
// configured default fileName when key is empty).
func (s *source) Load(ctx context.Context, key string) ([]byte, error) {
	fileName := s.resolveFileName(key)
	configFile, err := s.client.GetConfigFile(s.options.namespace, s.options.fileGroup, fileName)
	if err != nil {
		log.Printf("polaris fail to get config: %v", err)
		return nil, err
	}
	s.options.configFile = configFile
	return []byte(configFile.GetContent()), nil
}

// WatchValue implements [baseConfig.ValueWatcher].
// It returns a channel that receives the new value each time the configuration
// file for the given fileName (or the configured default) changes.
func (s *source) WatchValue(ctx context.Context, key string) (<-chan []byte, error) {
	fileName := s.resolveFileName(key)

	configFile, err := s.client.GetConfigFile(s.options.namespace, s.options.fileGroup, fileName)
	if err != nil {
		return nil, err
	}

	out := make(chan []byte, 1)

	fullPath := getFullPath(s.options.namespace, s.options.fileGroup, fileName)
	eventChanMap[fullPath] = eventChan{
		closed: false,
		event:  make(chan model.ConfigFileChangeEvent),
	}

	configFile.AddChangeListener(func(event model.ConfigFileChangeEvent) {
		meta := event.ConfigFileMetadata
		fp := getFullPath(meta.GetNamespace(), meta.GetFileGroup(), meta.GetFileName())
		ec := eventChanMap[fp]
		if ec.closed {
			return
		}
		select {
		case ec.event <- event:
		case <-ctx.Done():
		}
	})

	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				ec := eventChanMap[fullPath]
				if !ec.closed {
					ec.closed = true
				}
				return
			case event := <-eventChanMap[fullPath].event:
				select {
				case out <- []byte(event.NewValue):
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}
