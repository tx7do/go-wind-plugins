package sqs

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKind(t *testing.T) {
	assert.Equal(t, "sqs", KindSQS)
}

func TestNewServer(t *testing.T) {
	srv := NewServer(
		WithEndpoint("http://127.0.0.1:9324"), WithCodec("json"),
	)
	assert.NotNil(t, srv)
	assert.Equal(t, "sqs", srv.Name())
	assert.False(t, srv.started.Load())
}

func TestEndpoint(t *testing.T) {
	srv := NewServer(
		WithEndpoint("http://127.0.0.1:9324"), WithCodec("json"),
	)
	assert.Equal(t, "", srv.Endpoint())
}

func TestStopBeforeStart(t *testing.T) {
	srv := NewServer(
		WithEndpoint("http://127.0.0.1:9324"), WithCodec("json"),
	)
	err := srv.Stop(context.Background())
	assert.Nil(t, err)
}
