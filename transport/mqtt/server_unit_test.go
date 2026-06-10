package mqtt

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKind(t *testing.T) {
	assert.Equal(t, "mqtt", KindMQTT)
}

func TestNewServer(t *testing.T) {
	srv := NewServer(
		WithAddress([]string{"127.0.0.1:1883"}), WithCodec("json"),
	)
	assert.NotNil(t, srv)
	assert.Equal(t, "mqtt", srv.Name())
	assert.False(t, srv.started.Load())
}

func TestEndpoint(t *testing.T) {
	srv := NewServer(
		WithAddress([]string{"127.0.0.1:1883"}), WithCodec("json"),
	)
	assert.Equal(t, "", srv.Endpoint())
}

func TestStopBeforeStart(t *testing.T) {
	srv := NewServer(
		WithAddress([]string{"127.0.0.1:1883"}), WithCodec("json"),
	)
	err := srv.Stop(context.Background())
	assert.Nil(t, err)
}
