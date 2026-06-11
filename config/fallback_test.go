package config

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// --- test helpers ---

// staticReader is a simple [Reader] that always returns the same data.
type staticReader struct {
	data []byte
	err  error
}

func (s *staticReader) Load(_ context.Context, _ string) ([]byte, error) {
	return s.data, s.err
}

// swapReader is a thread-safe [Reader] whose return value can be changed.
type swapReader struct {
	data atomic.Value // stores []byte
	err  error
}

func newSwapReader(data []byte) *swapReader {
	r := &swapReader{}
	r.data.Store(data)
	return r
}

func (s *swapReader) Load(_ context.Context, _ string) ([]byte, error) {
	return s.data.Load().([]byte), s.err
}

func (s *swapReader) set(data []byte) { s.data.Store(data) }

// closeTracker wraps a [staticReader] and records whether Close was called.
type closeTracker struct {
	staticReader
	closed bool
}

func (c *closeTracker) Close() error {
	c.closed = true
	return nil
}

// closeErrReader is a [Closer] that always returns an error.
type closeErrReader struct {
	staticReader
}

func (c *closeErrReader) Close() error { return errors.New("close error") }

// stubValueWatcher is a [ValueWatcher] that delivers values from a
// caller-controlled channel.
type stubValueWatcher struct {
	staticReader
	ch chan []byte
}

func (s *stubValueWatcher) WatchValue(_ context.Context, _ string) (<-chan []byte, error) {
	return s.ch, nil
}

// watchErrReader implements [ValueWatcher] but always returns an error.
type watchErrReader struct {
	staticReader
}

func (w *watchErrReader) WatchValue(_ context.Context, _ string) (<-chan []byte, error) {
	return nil, errors.New("watch error")
}

// --- NewFallbackReader ---

func TestNewFallbackReader_NoReaders(t *testing.T) {
	_, err := NewFallbackReader()
	if err == nil {
		t.Fatal("expected error when no readers are provided")
	}
}

func TestNewFallbackReader_OneReader(t *testing.T) {
	r := &staticReader{data: []byte("ok")}
	fb, err := NewFallbackReader(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fb == nil {
		t.Fatal("expected non-nil FallbackReader")
	}
}

// --- Load ---

func TestFallbackReader_Load_FirstSucceeds(t *testing.T) {
	fb, _ := NewFallbackReader(
		&staticReader{data: []byte("a")},
		&staticReader{data: []byte("b")},
	)

	data, err := fb.Load(context.Background(), "key")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "a" {
		t.Fatalf("expected %q, got %q", "a", data)
	}
}

func TestFallbackReader_Load_FallbackToSecond(t *testing.T) {
	fb, _ := NewFallbackReader(
		&staticReader{data: nil}, // nil data → skip
		&staticReader{data: []byte("b")},
	)

	data, err := fb.Load(context.Background(), "key")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "b" {
		t.Fatalf("expected %q, got %q", "b", data)
	}
}

func TestFallbackReader_Load_SkipErrors(t *testing.T) {
	fb, _ := NewFallbackReader(
		&staticReader{err: errors.New("a-fail")},
		&staticReader{data: []byte("b")},
	)

	data, err := fb.Load(context.Background(), "key")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "b" {
		t.Fatalf("expected %q, got %q", "b", data)
	}
}

func TestFallbackReader_Load_AllFail(t *testing.T) {
	fb, _ := NewFallbackReader(
		&staticReader{err: errors.New("a-fail")},
		&staticReader{err: errors.New("b-fail")},
	)

	_, err := fb.Load(context.Background(), "key")
	if err == nil {
		t.Fatal("expected error when all sources fail")
	}
}

func TestFallbackReader_Load_AllNil(t *testing.T) {
	fb, _ := NewFallbackReader(
		&staticReader{data: nil},
		&staticReader{data: nil},
	)

	_, err := fb.Load(context.Background(), "key")
	if err == nil {
		t.Fatal("expected error when all sources return nil")
	}
}

// --- Close ---

func TestFallbackReader_Close_AllClosers(t *testing.T) {
	a := &closeTracker{}
	b := &closeTracker{}

	fb, _ := NewFallbackReader(a, b)
	if err := fb.Close(); err != nil {
		t.Fatal(err)
	}
	if !a.closed || !b.closed {
		t.Fatal("expected all closers to be closed")
	}
}

func TestFallbackReader_Close_SkipsNonClosers(t *testing.T) {
	a := &staticReader{} // no Close method
	b := &closeTracker{}

	fb, _ := NewFallbackReader(a, b)
	if err := fb.Close(); err != nil {
		t.Fatal(err)
	}
	if !b.closed {
		t.Fatal("expected closer to be closed")
	}
}

func TestFallbackReader_Close_JoinErrors(t *testing.T) {
	fb, _ := NewFallbackReader(&closeErrReader{}, &closeErrReader{})
	err := fb.Close()
	if err == nil {
		t.Fatal("expected joined close error")
	}
}

// --- WatchValue ---

func TestFallbackReader_WatchValue_NoWatchers(t *testing.T) {
	fb, _ := NewFallbackReader(&staticReader{})
	_, err := fb.WatchValue(context.Background(), "key")
	if err == nil {
		t.Fatal("expected error when no sub-sources implement ValueWatcher")
	}
}

func TestFallbackReader_WatchValue_SingleWatcher(t *testing.T) {
	r := newSwapReader([]byte("v1"))
	ch := make(chan []byte, 1)
	w := &stubValueWatcher{staticReader: staticReader{data: r.data.Load().([]byte)}, ch: ch}
	// Override Load to delegate to swapReader.
	fb, _ := NewFallbackReader(&delegatingReader{r: r, vw: w})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out, err := fb.WatchValue(ctx, "key")
	if err != nil {
		t.Fatal(err)
	}

	// Change the value and push a notification.
	r.set([]byte("v2"))
	ch <- []byte("trigger")

	select {
	case data := <-out:
		if string(data) != "v2" {
			t.Fatalf("expected %q, got %q", "v2", data)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notification")
	}

	cancel()
}

func TestFallbackReader_WatchValue_MultipleWatchers(t *testing.T) {
	r1 := newSwapReader([]byte("a"))
	r2 := newSwapReader([]byte("b"))
	ch1 := make(chan []byte, 1)
	ch2 := make(chan []byte, 1)

	w1 := &stubValueWatcher{ch: ch1}
	w2 := &stubValueWatcher{ch: ch2}

	fb, _ := NewFallbackReader(
		&delegatingReader{r: r1, vw: w1},
		&delegatingReader{r: r2, vw: w2},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out, err := fb.WatchValue(ctx, "key")
	if err != nil {
		t.Fatal(err)
	}

	// Change lower-priority source — effective value should still come
	// from the higher-priority source.
	r2.set([]byte("b-updated"))
	ch2 <- []byte("trigger")

	select {
	case data := <-out:
		// r1 still returns "a" so the effective value is "a".
		if string(data) != "a" {
			t.Fatalf("expected effective value %q, got %q", "a", data)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notification")
	}

	// Now change the high-priority source.
	r1.set([]byte("a-updated"))
	ch1 <- []byte("trigger")

	select {
	case data := <-out:
		if string(data) != "a-updated" {
			t.Fatalf("expected %q, got %q", "a-updated", data)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notification")
	}

	cancel()
}

func TestFallbackReader_WatchValue_WatchErr(t *testing.T) {
	fb, _ := NewFallbackReader(&watchErrReader{})
	_, err := fb.WatchValue(context.Background(), "key")
	if err == nil {
		t.Fatal("expected error when sub-source WatchValue fails")
	}
}

func TestFallbackReader_WatchValue_ContextCancel(t *testing.T) {
	ch := make(chan []byte, 1)
	w := &stubValueWatcher{staticReader: staticReader{data: []byte("v")}, ch: ch}
	fb, _ := NewFallbackReader(w)

	ctx, cancel := context.WithCancel(context.Background())

	out, err := fb.WatchValue(ctx, "key")
	if err != nil {
		t.Fatal(err)
	}

	// Cancel the context and verify the channel is closed.
	cancel()

	select {
	case _, ok := <-out:
		if ok {
			t.Fatal("expected channel to be closed after context cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for channel close")
	}
}

// delegatingReader combines a [Reader] and a [ValueWatcher] for tests
// where Load should delegate to a mutable reader.
type delegatingReader struct {
	r  *swapReader
	vw ValueWatcher
}

func (d *delegatingReader) Load(ctx context.Context, key string) ([]byte, error) {
	return d.r.Load(ctx, key)
}

func (d *delegatingReader) WatchValue(ctx context.Context, key string) (<-chan []byte, error) {
	return d.vw.WatchValue(ctx, key)
}
