package nsq

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tx7do/go-wind-plugins/broker"
)

// TestTLSOptionsWiredIntoGoNsqConfig 断言 broker 的 Secure/TLSConfig 选项
// 被装配进 go-nsq 客户端配置（TlsV1 开启 TLS 协商，TlsConfig 为具体配置）。
func TestTLSOptionsWiredIntoGoNsqConfig(t *testing.T) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}

	// Secure + TLSConfig：两者都生效。
	b := NewBroker(broker.WithEnableSecure(true), broker.WithTLSConfig(tlsCfg)).(*nsqBroker)
	require.NoError(t, b.Init())
	assert.True(t, b.config.TlsV1, "Secure must enable go-nsq TLS negotiation (TlsV1)")
	assert.Same(t, tlsCfg, b.config.TlsConfig, "TLSConfig must be handed to go-nsq as-is")

	// 仅 WithTLSConfig：broker.WithTLSConfig 会隐式置位 Secure。
	onlyCfg := NewBroker(broker.WithTLSConfig(tlsCfg)).(*nsqBroker)
	require.NoError(t, onlyCfg.Init())
	assert.True(t, onlyCfg.config.TlsV1)
	assert.Same(t, onlyCfg.config.TlsConfig, tlsCfg)

	// 默认不启用 TLS。
	def := NewBroker().(*nsqBroker)
	require.NoError(t, def.Init())
	assert.False(t, def.config.TlsV1)
	assert.Nil(t, def.config.TlsConfig)

	// 显式 nil TLSConfig 不启用 TLS。
	nilCfg := NewBroker(broker.WithTLSConfig(nil)).(*nsqBroker)
	require.NoError(t, nilCfg.Init())
	assert.False(t, nilCfg.config.TlsV1)
	assert.Nil(t, nilCfg.config.TlsConfig)
}
