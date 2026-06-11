package config

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// FallbackReader tries multiple [Reader] sources in priority order.
//
// On Load, each source is tried sequentially; the first one that returns
// non-nil data without error wins. This implements a cascading-fallback
// pattern where higher-priority sources shadow lower-priority ones.
//
// FallbackReader also implements:
//   - [Closer]       — closes all sub-sources that implement [Closer].
//   - [ValueWatcher] — merges change notifications from all sub-sources
//     that implement [ValueWatcher]. When any sub-source pushes a change,
//     the effective value is re-read via Load (respecting priority) and
//     forwarded to the merged output channel.
type FallbackReader struct {
	readers []Reader
}

// NewFallbackReader creates a [FallbackReader] from the given readers.
// Readers are tried in the order provided; the first reader has the
// highest priority. At least one reader must be provided.
func NewFallbackReader(readers ...Reader) (*FallbackReader, error) {
	if len(readers) == 0 {
		return nil, errors.New("fallback: at least one reader is required")
	}
	return &FallbackReader{readers: readers}, nil
}

// Load implements [Reader]. It tries each sub-source in priority order
// and returns the data from the first source that succeeds with non-nil
// data. If all sources fail or return nil, a combined error is returned.
func (f *FallbackReader) Load(ctx context.Context, key string) ([]byte, error) {
	var errs []error
	for _, r := range f.readers {
		data, err := r.Load(ctx, key)
		if err == nil && data != nil {
			return data, nil
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return nil, fmt.Errorf("fallback: no source could resolve key %q", key)
}

// Close implements [Closer]. It closes all sub-sources that implement
// [Closer] and returns the joined error (nil when there are no errors).
func (f *FallbackReader) Close() error {
	var errs []error
	for _, r := range f.readers {
		if c, ok := r.(Closer); ok {
			if err := c.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// WatchValue implements [ValueWatcher]. It watches all sub-sources that
// implement [ValueWatcher] and merges their notifications into a single
// channel. When any sub-source detects a change, the effective value is
// re-read via Load to respect priority order and forwarded.
//
// The returned channel is closed when ctx is cancelled or all sub-source
// watchers finish.
func (f *FallbackReader) WatchValue(ctx context.Context, key string) (<-chan []byte, error) {
	// Discover sub-sources that support ValueWatcher.
	var watchers []ValueWatcher
	for _, r := range f.readers {
		if vw, ok := r.(ValueWatcher); ok {
			watchers = append(watchers, vw)
		}
	}
	if len(watchers) == 0 {
		return nil, fmt.Errorf("fallback: none of the sub-sources implement ValueWatcher")
	}

	// Collect all sub-channels before starting goroutines so that a
	// failure to start one watcher does not leak goroutines.
	var subs []<-chan []byte
	for _, w := range watchers {
		ch, err := w.WatchValue(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("fallback: sub-source WatchValue failed: %w", err)
		}
		if ch != nil {
			subs = append(subs, ch)
		}
	}
	if len(subs) == 0 {
		return nil, fmt.Errorf("fallback: all sub-source WatchValue returned nil channels")
	}

	out := make(chan []byte, 1)
	var wg sync.WaitGroup

	for _, ch := range subs {
		wg.Add(1)
		go func(ch <-chan []byte) {
			defer wg.Done()
			for {
				select {
				case _, ok := <-ch:
					if !ok {
						return
					}
					// Re-read to get the effective value respecting priority.
					effective, err := f.Load(ctx, key)
					if err != nil || effective == nil {
						continue
					}
					select {
					case out <- effective:
					case <-ctx.Done():
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}(ch)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out, nil
}

// Compile-time interface assertions.
var (
	_ Reader       = (*FallbackReader)(nil)
	_ Closer       = (*FallbackReader)(nil)
	_ ReadCloser   = (*FallbackReader)(nil)
	_ ValueWatcher = (*FallbackReader)(nil)
)
