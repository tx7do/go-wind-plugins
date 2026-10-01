package workflow

import (
	"errors"
	"testing"
)

// mockClient is a minimal Client implementation for testing.
type mockClient struct {
	closed   bool
	closeErr error
	closeNbr int
}

func (m *mockClient) Close() error {
	m.closeNbr++
	m.closed = true
	return m.closeErr
}

// mockWorker is a minimal Worker implementation for testing.
type mockWorker struct {
	stopped bool
	running bool
}

func (m *mockWorker) Stop()           { m.stopped = true }
func (m *mockWorker) IsRunning() bool { return m.running }

// ---------------------------------------------------------------------------
// Interface compliance
// ---------------------------------------------------------------------------

var (
	_ Client = (*mockClient)(nil)
	_ Worker = (*mockWorker)(nil)
)

func TestClientInterface(t *testing.T) {
	c := &mockClient{}

	if err := c.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
	if !c.closed {
		t.Error("Close() should mark the client as closed")
	}
	if c.closeNbr != 1 {
		t.Errorf("Close() call count = %d, want 1", c.closeNbr)
	}
}

func TestClientInterface_CloseError(t *testing.T) {
	wantErr := errors.New("connection already closed")
	c := &mockClient{closeErr: wantErr}

	if err := c.Close(); !errors.Is(err, wantErr) {
		t.Errorf("Close() error = %v, want %v", err, wantErr)
	}
}

func TestWorkerInterface(t *testing.T) {
	w := &mockWorker{running: true}

	if !w.IsRunning() {
		t.Error("IsRunning() = false, want true")
	}

	w.Stop()
	if !w.stopped {
		t.Error("Stop() should mark the worker as stopped")
	}

	w.running = false
	if w.IsRunning() {
		t.Error("IsRunning() = true, want false after stopping")
	}
}
