package redis

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKind(t *testing.T) {
	assert.Equal(t, "redis", KindRedis)
}

func TestNewServer(t *testing.T) {
	srv := NewServer(
		WithAddress("redis://127.0.0.1:6379"), WithCodec("json"),
	)
	assert.NotNil(t, srv)
	assert.Equal(t, "redis", srv.Name())
	assert.False(t, srv.started.Load())
}

func TestEndpoint(t *testing.T) {
	srv := NewServer(
		WithAddress("redis://127.0.0.1:6379"), WithCodec("json"),
	)
	assert.Equal(t, "", srv.Endpoint())
}

func TestStopBeforeStart(t *testing.T) {
	srv := NewServer(
		WithAddress("redis://127.0.0.1:6379"), WithCodec("json"),
	)
	err := srv.Stop(context.Background())
	assert.Nil(t, err)
}
