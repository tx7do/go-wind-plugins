package cassandra

import (
	"errors"
	"testing"

	"github.com/gocql/gocql"
	"github.com/stretchr/testify/assert"
)

// TestClient_NilReceiver verifies the nil-receiver guards: Close, Closed and
// Session tolerate a nil *Client.
func TestClient_NilReceiver(t *testing.T) {
	var c *Client
	assert.NotPanics(t, func() { c.Close() })
	assert.True(t, c.Closed())
	assert.Nil(t, c.Session())
}

// TestClient_ZeroClient_SessionReturnsNil checks the Session accessor on a
// non-nil client without a session.
func TestClient_ZeroClient_SessionReturnsNil(t *testing.T) {
	c := &Client{}
	assert.True(t, c.Closed())
	assert.Nil(t, c.Session())
}

// TestClient_ZeroSession_NoDialGuards exercises the statement-validation
// guards with a zero-value gocql.Session (which reports itself as open, so
// the wrapper proceeds to argument validation and must reject bad statements
// before any query execution happens).
func TestClient_ZeroSession_NoDialGuards(t *testing.T) {
	c := &Client{session: &gocql.Session{}}
	assert.False(t, c.Closed(), "a zero gocql.Session reports itself as open")
	assert.Same(t, c.session, c.Session())

	// Empty statements must be rejected before execution.
	assert.True(t, errors.Is(c.Exec(nil, ""), ErrInvalidRequest))
	_, err := c.Query(nil, "")
	assert.True(t, errors.Is(err, ErrInvalidRequest))

	// Batch guards: empty statement list and empty statement string.
	assert.True(t, errors.Is(
		c.ExecBatch(nil, gocql.LoggedBatch, nil, nil), ErrInvalidRequest))
	assert.True(t, errors.Is(
		c.ExecBatch(nil, gocql.LoggedBatch, []string{""}, [][]any{{}}), ErrInvalidRequest))
	assert.True(t, errors.Is(
		c.ExecBatch(nil, gocql.LoggedBatch, []string{"ok", ""}, [][]any{{}, {}}), ErrInvalidRequest))

	assert.True(t, errors.Is(
		c.ExecBatchBound(nil, gocql.LoggedBatch, nil, nil), ErrInvalidRequest))
	assert.True(t, errors.Is(
		c.ExecBatchBound(nil, gocql.LoggedBatch, []string{""}, nil), ErrInvalidRequest))
	assert.True(t, errors.Is(
		c.ExecBatchBound(nil, gocql.LoggedBatch, []string{"ok", ""},
			[]func(*gocql.QueryInfo) ([]any, error){nil, nil}), ErrInvalidRequest))
}

// TestErrorSentinels pins the sentinel identities so callers matching with
// errors.Is stay stable.
func TestErrorSentinels(t *testing.T) {
	assert.NotNil(t, ErrInvalidRequest)
	assert.NotNil(t, ErrExecQuery)
	assert.NotNil(t, ErrSessionClosed)

	// Each sentinel must be distinct.
	assert.False(t, errors.Is(ErrInvalidRequest, ErrExecQuery))
	assert.False(t, errors.Is(ErrExecQuery, ErrSessionClosed))
	assert.False(t, errors.Is(ErrSessionClosed, ErrInvalidRequest))
}
