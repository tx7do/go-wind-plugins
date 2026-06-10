package file

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"

	"github.com/tx7do/go-wind/log"

	baseConfig "github.com/tx7do/go-wind-plugins/config"
)

var (
	_ baseConfig.Reader       = (*source)(nil)
	_ baseConfig.ValueWatcher = (*source)(nil)
	_ baseConfig.Closer       = (*source)(nil)
)

type source struct {
	options *options
	watcher *fsnotify.Watcher
}

// New creates a file-based config source.
// The path option is required; an error is returned if it is empty.
func New(opts ...Option) (*source, error) {
	o := &options{
		ctx: context.Background(),
	}
	for _, opt := range opts {
		opt(o)
	}

	if o.path == "" {
		return nil, errors.New("path invalid")
	}

	// Resolve to absolute path so that fsnotify events match.
	abs, err := filepath.Abs(o.path)
	if err != nil {
		return nil, fmt.Errorf("resolve absolute path: %w", err)
	}
	o.path = abs

	s := &source{options: o}

	if o.watch {
		w, err := fsnotify.NewWatcher()
		if err != nil {
			return nil, fmt.Errorf("create fsnotify watcher: %w", err)
		}
		s.watcher = w

		// Watch the parent directory — file-level watches are lost on atomic
		// renames (editors often write to a temp file then rename).
		dir := filepath.Dir(o.path)
		if err := w.Add(dir); err != nil {
			_ = w.Close()
			return nil, fmt.Errorf("watch directory %s: %w", dir, err)
		}

		log.GetLogger().Debug(context.Background(), "[file] watching", "dir", dir, "file", o.path)
	}

	return s, nil
}

// resolveKey returns the key to use for the given caller-provided key.
// If key is empty the configured default path is used.
func (s *source) resolveKey(key string) string {
	if key != "" {
		abs, err := filepath.Abs(key)
		if err != nil {
			return key
		}
		return abs
	}
	return s.options.path
}

// Load implements [baseConfig.Reader].
// It reads the entire file at key (or the configured default path when key
// is empty) and returns its raw contents.
func (s *source) Load(_ context.Context, key string) ([]byte, error) {
	path := s.resolveKey(key)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", path, err)
	}
	return data, nil
}

// WatchValue implements [baseConfig.ValueWatcher].
// It returns a channel that receives the new file contents each time the file
// at key (or the configured default path) is modified or created.
// The channel is closed when ctx is cancelled or the watcher encounters a
// fatal error.
func (s *source) WatchValue(ctx context.Context, key string) (<-chan []byte, error) {
	path := s.resolveKey(key)

	if s.watcher == nil {
		// Lazy-init a watcher if the source was created without watch enabled.
		w, err := fsnotify.NewWatcher()
		if err != nil {
			return nil, fmt.Errorf("create fsnotify watcher: %w", err)
		}
		dir := filepath.Dir(path)
		if err := w.Add(dir); err != nil {
			_ = w.Close()
			return nil, fmt.Errorf("watch directory %s: %w", dir, err)
		}
		s.watcher = w
	}

	out := make(chan []byte, 1)

	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-s.watcher.Events:
				if !ok {
					return
				}
				// Only react to events for our target file.
				if event.Name != path {
					continue
				}
				if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
					continue
				}
				data, err := os.ReadFile(path)
				if err != nil {
					log.GetLogger().Debug(context.Background(), "[file] read after event failed", "path", path, "error", err)
					continue
				}
				select {
				case out <- data:
				case <-ctx.Done():
					return
				}
			case err, ok := <-s.watcher.Errors:
				if !ok {
					return
				}
				log.GetLogger().Error(context.Background(), "[file] watcher error", "error", err)
				return
			}
		}
	}()

	return out, nil
}

// Close releases the fsnotify watcher.
func (s *source) Close() error {
	if s.watcher != nil {
		return s.watcher.Close()
	}
	return nil
}
