package nsq

import (
	"crypto/tls"

	"github.com/tx7do/go-wind-plugins/broker"
	"github.com/tx7do/go-wind-plugins/broker/nsq"
	"github.com/tx7do/go-wind-plugins/metrics"
)

type ServerOption func(o *Server)

// WithBrokerOptions MQ代理配置
func WithBrokerOptions(opts ...broker.Option) ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, opts...)
	}
}

func WithAddress(addrs []string) ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, broker.WithAddress(addrs...))
	}
}

func WithLookupdAddress(addrs []string) ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, nsq.WithLookupdAddress(addrs))
	}
}

// WithTLSConfig TLS配置：透传给 broker 驱动并作用于 go-nsq 客户端——
// Secure=true 开启 TLS 协商（IDENTIFY tls_v1），TLSConfig 作为具体 TLS 配置。
// 要求 nsqd/nslookupd 端开启 TLS，否则协商回退明文。
func WithTLSConfig(c *tls.Config) ServerOption {
	return func(s *Server) {
		if c != nil {
			s.brokerOpts = append(s.brokerOpts, broker.WithEnableSecure(true))
		}
		s.brokerOpts = append(s.brokerOpts, broker.WithTLSConfig(c))
	}
}

func WithCodec(c string) ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, broker.WithCodec(c))
	}
}

func WithConsumerOptions(opts []string) ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, nsq.WithConsumerOptions(opts))
	}
}

// WithMetrics 注入指标监控
func WithMetrics(m metrics.Metrics) ServerOption {
	return func(s *Server) {
		s.m = m
	}
}
