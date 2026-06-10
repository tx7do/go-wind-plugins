package gcpubsub

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKind(t *testing.T) {
	assert.Equal(t, "gcpubsub", KindGCPPubSub)
}

func TestNewServer(t *testing.T) {
	srv := NewServer(
		WithProjectID("test"), WithEndpoint("localhost"), WithCodec("json"),
	)
	assert.NotNil(t, srv)
	assert.Equal(t, "gcpubsub", srv.Name())
	assert.False(t, srv.started.Load())
}

func TestEndpoint(t *testing.T) {
	srv := NewServer(
		WithProjectID("test"), WithEndpoint("localhost"), WithCodec("json"),
	)
	assert.Equal(t, "", srv.Endpoint())
}

func TestStopBeforeStart(t *testing.T) {
	srv := NewServer(
		WithProjectID("test"), WithEndpoint("localhost"), WithCodec("json"),
	)
	err := srv.Stop(context.Background())
	assert.Nil(t, err)
}
