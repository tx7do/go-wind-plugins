// Package main demonstrates how to integrate OpenTelemetry tracing into an
// HTTP server using the go-wind tracer/otlp plugin and the HTTP tracing
// middleware.
//
// It starts an HTTP server on :8080 where every request gets an OTel server
// span. The example uses a noop TracerProvider (no real collector) so it can
// run standalone. To export real traces, replace the noop provider with
// tracer/otlp.New().
//
// Run:
//
//	go run ./examples/tracer-basic
//
// Test:
//
//	curl http://localhost:8080/hello
//	curl http://localhost:8080/error
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	tracerOtlp "github.com/tx7do/go-wind-plugins/tracer/otlp"
	httpServer "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/driver/std"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/logging"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/recovery"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/tracing"
)

func main() {
	// ---------------------------------------------------------------
	// 1. Initialise OpenTelemetry
	// ---------------------------------------------------------------
	// In production you would use the OTLP exporter:
	//
	//   tp, err := tracerOtlp.New(
	//       tracerOtlp.WithEndpoint("localhost:4317"),
	//       tracerOtlp.WithServiceName("my-service"),
	//       tracerOtlp.WithInsecure(true),
	//   )
	//   if err != nil { log.Fatal(err) }
	//   defer tp.Shutdown(context.Background())
	//
	// For this self-contained example we use a noop provider so no
	// collector is needed.
	otel.SetTracerProvider(trace.NewNoopTracerProvider())

	// You can also use the Tracer helper for custom spans in business code:
	tr := tracerOtlp.NewTracer(trace.SpanKindClient, "example-call")

	// ---------------------------------------------------------------
	// 2. Create HTTP server with tracing middleware
	// ---------------------------------------------------------------
	srv := httpServer.NewServer(":8080", httpServer.WithDriver(std.NewDriver()))

	srv.Use(
		recovery.Middleware(),
		logging.Middleware(),
		tracing.Middleware(), // creates an OTel span for every request
	)

	// Hello endpoint — a normal 200 response.
	srv.GET("/hello", func(w http.ResponseWriter, r *http.Request) {
		// You can access the current span from the context if needed:
		span := trace.SpanFromContext(r.Context())
		span.AddEvent("processing hello request")

		fmt.Fprintf(w, "Hello! TraceID: %s\n", span.SpanContext().TraceID())
	})

	// Error endpoint — returns 500, the tracing middleware will mark the
	// span as Error.
	srv.GET("/error", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "something went wrong", http.StatusInternalServerError)
	})

	// Custom-span endpoint — demonstrates using the Tracer wrapper to
	// create a child span for an outgoing or internal operation.
	srv.GET("/work", func(w http.ResponseWriter, r *http.Request) {
		// Start a child span using the Tracer helper.
		carrier := propagation.HeaderCarrier(r.Header)
		ctx, span := tr.Start(r.Context(), carrier)
		defer tr.End(ctx, span, nil)

		fmt.Fprintln(w, "work done (child span created)")
	})

	// ---------------------------------------------------------------
	// 3. Start the server
	// ---------------------------------------------------------------
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("HTTP server with tracing listening on %s\n", srv.Endpoint())
	fmt.Println("Try: curl http://localhost:8080/hello")
	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("server stopped")
}
