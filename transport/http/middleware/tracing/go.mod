module github.com/tx7do/go-wind-plugins/transport/http/middleware/tracing

go 1.26.3

require (
	github.com/tx7do/go-wind-plugins/transport/http v0.0.0
	go.opentelemetry.io/otel v1.43.0
	go.opentelemetry.io/otel/trace v1.43.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/tx7do/go-wind v0.0.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/metric v1.43.0 // indirect
)

replace github.com/tx7do/go-wind-plugins/transport/http => ../..
