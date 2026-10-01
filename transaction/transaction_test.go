package transaction_test

import (
	"errors"
	"testing"

	"github.com/tx7do/go-wind-plugins/transaction"
)

// mockClient is a minimal implementation of transaction.Client used to
// verify the interface contract from the outside.
type mockClient struct {
	closed     bool
	closeErr   error
	closeCalls int
}

func (m *mockClient) Close() error {
	m.closeCalls++
	m.closed = true
	return m.closeErr
}

// ---------------------------------------------------------------------------
// Interface compliance
// ---------------------------------------------------------------------------

// Compile-time assertions: any engine client must satisfy transaction.Client.
var (
	_ transaction.Client = (*mockClient)(nil)
	_ transaction.Client = mockCloseNoop{}
)

// mockCloseNoop has a value receiver, so both the value and the pointer
// satisfy the interface.
type mockCloseNoop struct{}

func (mockCloseNoop) Close() error { return nil }

func TestClientInterface_MockImplementation(t *testing.T) {
	var c transaction.Client = &mockClient{}

	if err := c.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

func TestClientInterface_ValueImplementation(t *testing.T) {
	var c transaction.Client = mockCloseNoop{}

	if err := c.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

func TestClientClose_PropagatesError(t *testing.T) {
	wantErr := errors.New("close failed")
	c := &mockClient{closeErr: wantErr}
	var client transaction.Client = c

	if err := client.Close(); !errors.Is(err, wantErr) {
		t.Errorf("Close() error = %v, want %v", err, wantErr)
	}
	if !c.closed {
		t.Error("Close() did not reach the implementation")
	}
	if c.closeCalls != 1 {
		t.Errorf("Close() called %d times, want 1", c.closeCalls)
	}
}
