package keto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NewClient with useGRPC=false only builds REST SDK configuration (no
// requests are sent), and with useGRPC=true it uses lazy gRPC connections
// (grpc.NewClient does not dial until the first RPC), so construction is
// hermetic. Requests themselves need a live server and stay behind the
// KRATOS_IT gate in keto_test.go.

func TestNewClient_REST(t *testing.T) {
	cli := NewClient("http://127.0.0.1:4466", "http://127.0.0.1:4467", false)
	require.NotNil(t, cli)
	assert.False(t, cli.useGRPC)
	assert.NotNil(t, cli.readClient, "REST read client should be built")
	assert.NotNil(t, cli.writeClient, "REST write client should be built")
}

func TestNewClient_GRPC(t *testing.T) {
	cli := NewClient("127.0.0.1:4466", "127.0.0.1:4467", true)
	require.NotNil(t, cli)
	assert.True(t, cli.useGRPC)
	assert.NotNil(t, cli.checkServiceClient)
	assert.NotNil(t, cli.readServiceClient)
	assert.NotNil(t, cli.writeServiceClient)
	assert.NotNil(t, cli.expandServiceClient)
}
