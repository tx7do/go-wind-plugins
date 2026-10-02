package rabbitmq

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tx7do/go-wind-plugins/broker"
)

// TestBrokerOptionsCarryTLSConfig 断言 broker 选项装配层携带 TLS 配置与 Secure 标志。
//
// 契约：Connect() 会把 Options().TLSConfig 装进 AMQP 连接配置
// （rabbitmq.go 中 conf.TLSClientConfig = b.options.TLSConfig），
// 并把 Options().Secure 传给底层连接（amqps）。本测试锁定选项装配，
// conf 映射由 Connect 对真实 AMQP 端点生效。
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
