package azuresb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKind(t *testing.T) {
	assert.Equal(t, "azuresb", KindAzureSB)
}

func TestNewServer(t *testing.T) {
	srv := NewServer(
		WithConnectionString("Endpoint=sb://localhost/;SharedAccessKeyName=test;SharedAccessKey=test"), WithCodec("json"),
	)
	assert.NotNil(t, srv)
	assert.Equal(t, "azuresb", srv.Name())
	assert.False(t, srv.started.Load())
}

func TestEndpoint(t *testing.T) {
	srv := NewServer(
		WithConnectionString("Endpoint=sb://localhost/;SharedAccessKeyName=test;SharedAccessKey=test"), WithCodec("json"),
	)
	assert.Equal(t, "", srv.Endpoint())
}

func TestStopBeforeStart(t *testing.T) {
	srv := NewServer(
		WithConnectionString("Endpoint=sb://localhost/;SharedAccessKeyName=test;SharedAccessKey=test"), WithCodec("json"),
	)
	err := srv.Stop(context.Background())
	assert.Nil(t, err)
}
