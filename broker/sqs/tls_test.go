package sqs

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"

	"github.com/tx7do/go-wind-plugins/broker"
)

// TestTLSHTTPClientCarriesConfig 断言 newTLSHTTPClient 把 *tls.Config
// 装进 AWS SDK HTTP 客户端底层 transport 的 TLSClientConfig。
func TestTLSHTTPClientCarriesConfig(t *testing.T) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}

	client := newTLSHTTPClient(tlsCfg)
	bc, ok := client.(*awshttp.BuildableClient)
	require.True(t, ok, "the AWS HTTP client must remain a BuildableClient")

	tr := bc.GetTransport()
	require.NotNil(t, tr)
	require.NotNil(t, tr.TLSClientConfig, "TLSClientConfig must be set on the transport")
	assert.Equal(t, uint16(tls.VersionTLS12), tr.TLSClientConfig.MinVersion)
	// 注入的是用户配置的克隆，用户侧后续修改不应影响已装配的客户端。
	assert.NotSame(t, tlsCfg, tr.TLSClientConfig)
}

// TestBrokerOptionsCarryTLSConfig 断言 broker 选项装配层携带 TLS 配置与 Secure 标志。
func TestBrokerOptionsCarryTLSConfig(t *testing.T) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}

	b := NewBroker(broker.WithTLSConfig(tlsCfg))
	opts := b.Options()
	assert.Same(t, tlsCfg, opts.TLSConfig)
	assert.True(t, opts.Secure, "broker.WithTLSConfig must imply Secure")

	plain := NewBroker()
	assert.Nil(t, plain.Options().TLSConfig)
	assert.False(t, plain.Options().Secure)

	nilCfg := NewBroker(broker.WithTLSConfig(nil))
	assert.Nil(t, nilCfg.Options().TLSConfig)
	assert.False(t, nilCfg.Options().Secure)
}

// TestConnectWithTLSConfigHermetic 断言携带 TLS 配置的 Connect 全流程
// 无需真实 AWS 环境即可完成（仅装配客户端，不发起网络请求）。
func TestConnectWithTLSConfigHermetic(t *testing.T) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}

	b := NewBroker(
		broker.WithTLSConfig(tlsCfg),
		WithRegion("us-east-1"),
		WithEndpoint("http://127.0.0.1:9324"),
	).(*sqsBroker)

	require.NoError(t, b.Init())
	require.NoError(t, b.Connect())
	require.NotNil(t, b.client)

	// Connect 建立的客户端已带上自定义 TLS transport。
	bc, ok := b.client.Options().HTTPClient.(*awshttp.BuildableClient)
	if ok {
		require.NotNil(t, bc.GetTransport().TLSClientConfig)
		assert.Equal(t, uint16(tls.VersionTLS12), bc.GetTransport().TLSClientConfig.MinVersion)
	}

	require.NoError(t, b.Disconnect())
	assert.Nil(t, b.client)
}
