package config

import (
	"context"
	"testing"
)

// --- compile-time interface assertions ---

type mockReader struct{}

func (mockReader) Load(context.Context, string) ([]byte, error) { return nil, nil }

type mockCloser struct{}

func (mockCloser) Close() error { return nil }

type mockReadCloser struct{}

func (mockReadCloser) Load(context.Context, string) ([]byte, error) { return nil, nil }
func (mockReadCloser) Close() error                                 { return nil }

type mockWatcher struct{}

func (mockWatcher) Watch(context.Context, string) (<-chan struct{}, error) {
	return nil, nil
}

type mockReadWatcher struct{}

func (mockReadWatcher) Load(context.Context, string) ([]byte, error) { return nil, nil }
func (mockReadWatcher) Watch(context.Context, string) (<-chan struct{}, error) {
	return nil, nil
}

type mockValueWatcher struct{}

func (mockValueWatcher) WatchValue(context.Context, string) (<-chan []byte, error) {
	return nil, nil
}

type mockDecoder struct{}

func (mockDecoder) Decode([]byte, any) error { return nil }

var (
	_ Reader       = mockReader{}
	_ Closer       = mockCloser{}
	_ ReadCloser   = mockReadCloser{}
	_ Watcher      = mockWatcher{}
	_ ReadWatcher  = mockReadWatcher{}
	_ ValueWatcher = mockValueWatcher{}
	_ Decoder      = mockDecoder{}
)

// --- functional tests ---

func TestInterfaces_Composable(t *testing.T) {
	// A provider that implements both ReadCloser and Watcher also satisfies
	// ReadWatcher via embedding, but the converse is not required.
	var rc ReadCloser = mockReadCloser{}
	_ = rc

	var w Watcher = mockWatcher{}
	_ = w

	// ValueWatcher is independent — a provider may implement it without
	// implementing Watcher.
	var vw ValueWatcher = mockValueWatcher{}
	_ = vw

	// Decoder is independent of Reader/Watcher.
	var d Decoder = mockDecoder{}
	_ = d
}
