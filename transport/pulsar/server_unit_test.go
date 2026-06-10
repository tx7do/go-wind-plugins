package pulsar

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKind(t *testing.T) {
	assert.Equal(t, "pulsar", KindPulsar)
}

func TestNewServer(t *testing.T) {
	srv := NewServer(
		WithAddress([]string{"pulsar://127.0.0.1:6650"}), WithCodec("json"),
	)
	assert.NotNil(t, srv)
	assert.Equal(t, "pulsar", srv.Name())
	assert.False(t, srv.started.Load())
}

func TestEndpoint(t *testing.T) {
	srv := NewServer(
		WithAddress([]string{"pulsar://127.0.0.1:6650"}), WithCodec("json"),
	)
	assert.Equal(t, "", srv.Endpoint())
}

func TestStopBeforeStart(t *testing.T) {
	srv := NewServer(
		WithAddress([]string{"pulsar://127.0.0.1:6650"}), WithCodec("json"),
	)
	err := srv.Stop(context.Background())
	assert.Nil(t, err)
}
