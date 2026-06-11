package gcpubsub

import (
	"crypto/tls"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/tx7do/go-wind-plugins/broker"
	"github.com/tx7do/go-wind-plugins/broker/gcpubsub"
	"github.com/tx7do/go-wind-plugins/metrics"
)

type ServerOption func(o *Server)

// WithBrokerOptions sets broker options.
func WithBrokerOptions(opts ...broker.Option) ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, opts...)
	}
}

// WithProjectID sets the GCP project ID.
func WithProjectID(projectID string) ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, gcpubsub.WithProjectID(projectID))
	}
}

// WithCredentialsFile sets the path to a service account credentials JSON file.
func WithCredentialsFile(path string) ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, gcpubsub.WithCredentialsFile(path))
	}
}

// WithEndpoint sets a custom endpoint (for testing with emulators).
func WithEndpoint(endpoint string) ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, gcpubsub.WithEndpoint(endpoint))
	}
}

// WithCodec sets the codec for message serialization.
func WithCodec(c string) ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, broker.WithCodec(c))
	}
}

// WithTLSConfig TLS配置
func WithTLSConfig(c *tls.Config) ServerOption {
	return func(s *Server) {
		if c != nil {
			s.brokerOpts = append(s.brokerOpts, broker.WithEnableSecure(true))
		}
		s.brokerOpts = append(s.brokerOpts, broker.WithTLSConfig(c))
	}
}

// WithGlobalTracerProvider 注入全局的链路追踪器的Provider
func WithGlobalTracerProvider() ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, broker.WithGlobalTracerProvider())
	}
}

// WithGlobalPropagator 注入全局的链路追踪器的Propagator
func WithGlobalPropagator() ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, broker.WithGlobalPropagator())
	}
}

// WithTracerProvider 注入链路追踪器的Provider
func WithTracerProvider(provider trace.TracerProvider, _ string) ServerOption {
	return func(s *Server) {
		s.brokerOpts = append(s.brokerOpts, broker.WithTracerProvider(provider))
	}
}

// WithPropagator 注入链路追踪器的Propagator
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
