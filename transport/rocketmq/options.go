package rocketmq

import (
	"crypto/tls"

	rocketmqOption "github.com/tx7do/go-wind-plugins/broker/rocketmq/option"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/tx7do/go-wind-plugins/broker"
	"github.com/tx7do/go-wind-plugins/metrics"
)

type ServerOption func(o *Server)

// WithBrokerOptions MQ代理配置
func WithBrokerOptions(opts ...broker.Option) ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, opts...)
	}
}

// WithTLSConfig 注意：当前 rocketmq broker 驱动（aliyun HTTP SDK / v2 客户端 /
// v5 客户端）均不消费该 TLS 配置，也不消费 Secure 标志——v5 SDK 虽有连接级
// WithTLSConfig 选项，但其 client manager 创建 gRPC 连接时不透传任何选项，
// 自定义 *tls.Config 无法送达。本选项仅为 API 兼容而保留。
func WithTLSConfig(c *tls.Config) ServerOption {
	return func(s *Server) {
		if c != nil {
			s.brokerOpts = append(s.brokerOpts, broker.WithEnableSecure(true))
		}
		s.brokerOpts = append(s.brokerOpts, broker.WithTLSConfig(c))
	}
}

func WithEnableTrace() ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, rocketmqOption.WithEnableTrace())
	}
}

func WithNameServer(addrs []string) ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, rocketmqOption.WithNameServer(addrs))
	}
}

func WithNameServerDomain(uri string) ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, rocketmqOption.WithNameServerDomain(uri))
	}
}

func WithCredentials(accessKey, secretKey, securityToken string) ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, rocketmqOption.WithCredentials(accessKey, secretKey, securityToken))
	}
}

func WithNamespace(ns string) ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, rocketmqOption.WithNamespace(ns))
	}
}

func WithInstanceName(name string) ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, rocketmqOption.WithInstanceName(name))
	}
}

func WithGroupName(name string) ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, rocketmqOption.WithGroupName(name))
	}
}

func WithRetryCount(count int) ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, rocketmqOption.WithRetryCount(count))
	}
}

func WithCodec(c string) ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, broker.WithCodec(c))
	}
}

func WithGlobalTracerProvider() ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, broker.WithGlobalTracerProvider())
	}
}

func WithGlobalPropagator() ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, broker.WithGlobalPropagator())
	}
}

func WithTracerProvider(provider trace.TracerProvider, _ string) ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, broker.WithTracerProvider(provider))
	}
}

func WithPropagator(propagators propagation.TextMapPropagator) ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, broker.WithPropagator(propagators))
	}
}

// WithMetrics 注入指标监控
func WithMetrics(m metrics.Metrics) ServerOption {
	return func(s *Server) {
		s.m = m
	}
}
