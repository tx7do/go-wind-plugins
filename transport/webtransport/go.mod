module github.com/tx7do/go-wind-plugins/transport/webtransport

go 1.26.3

replace (
	github.com/tx7do/go-wind-plugins/broker => ../../broker
	github.com/tx7do/go-wind-plugins/encoding => ../../encoding
	github.com/tx7do/go-wind-plugins/transport => ../
)

require (
	github.com/quic-go/quic-go v0.59.0
	github.com/tx7do/go-wind v0.0.1
	github.com/tx7do/go-wind-plugins/broker v0.0.1
	github.com/tx7do/go-wind-plugins/encoding v0.0.1
	github.com/tx7do/go-wind-plugins/testing v0.0.0-20260610141552-e0f0a8ee1a9d
)

require (
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.29.0 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/tx7do/go-wind-plugins/encoding/json v0.0.1 // indirect
	github.com/tx7do/go-wind-plugins/encoding/proto v0.0.1 // indirect
	github.com/tx7do/go-wind-plugins/tracer/otlp v0.0.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.43.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.43.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.43.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.43.0 // indirect
	go.opentelemetry.io/otel/metric v1.43.0 // indirect
	go.opentelemetry.io/otel/sdk v1.43.0 // indirect
	go.opentelemetry.io/otel/trace v1.43.0 // indirect
	go.opentelemetry.io/proto/otlp v1.10.0 // indirect
	golang.org/x/crypto v0.50.0 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260427160629-7cedc36a6bc4 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260427160629-7cedc36a6bc4 // indirect
	google.golang.org/grpc v1.80.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)
