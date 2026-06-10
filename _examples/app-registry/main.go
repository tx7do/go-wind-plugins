// Package main demonstrates service registration with etcd using the go-wind
// [App] lifecycle hooks.
//
// This example builds on app-multi-server by adding:
//   - Service registration to etcd on startup (via AfterStart logic)
//   - Service deregistration on graceful shutdown (via BeforeStop hook)
//   - A /health endpoint for health-check integration
//
// Prerequisites:
//
//	# Start a local etcd server (Docker)
//	docker run -d --name etcd -p 2379:2379 gcr.io/etcd-development/etcd:v3.6.10 etcd
//
// Run:
//
//	go run ./examples/app-registry
//
// Test:
//
//	# HTTP health check
//	curl http://localhost:8080/health
//
//	# List registered services in etcd
//	etcdctl --endpoints=localhost:2379 get /microservices --prefix
//
//	# Observe deregistration on shutdown
//	# 1. Verify the key exists (above command)
//	# 2. Press Ctrl+C to stop the app
//	# 3. Run the etcdctl command again — the key should be gone
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/tx7do/go-wind"
	"github.com/tx7do/go-wind-plugins/registry/etcd"
	httpServer "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/driver/std"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/recovery"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/requestid"
	"go.etcd.io/etcd/client/v3"
)

func main() {
	// 1. Create the HTTP server.
	httpSrv := httpServer.NewServer(":8080",
		httpServer.WithDriver(std.NewDriver()),
	)
	httpSrv.Use(
		recovery.Middleware(),
		requestid.Middleware(),
	)
	httpSrv.GET("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status":"ok"}`)
	})

	// 2. Connect to etcd and create a Registrar.
	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to etcd: %v\n", err)
		fmt.Fprintln(os.Stderr, "Hint: start etcd with:")
		fmt.Fprintln(os.Stderr, "  docker run -d --name etcd -p 2379:2379 gcr.io/etcd-development/etcd:v3.6.10 etcd")
		os.Exit(1)
	}
	defer etcdClient.Close()

	reg := etcd.New(etcdClient,
		etcd.Namespace("/microservices"),
		etcd.RegisterTTL(15*time.Second),
	)

	// 3. Build the service instance metadata.
	//    Instance fields are exported, so we can construct it directly.
	const (
		serviceID   = "registry-demo-001"
		serviceName = "demo-service"
		serviceVer  = "1.0.0"
	)
	instance := &wind.Instance{
		ID:        serviceID,
		Name:      serviceName,
		Version:   serviceVer,
		Endpoints: []string{httpSrv.Endpoint()},
		Metadata:  map[string]string{"protocol": "http"},
	}

	// 4. Register on startup — BEFORE Run so the service is discoverable
	//    before it starts accepting traffic.
	ctx := context.Background()
	fmt.Printf("Registering service %q in etcd...\n", instance.Name)
	if err := reg.Register(ctx, instance); err != nil {
		fmt.Fprintf(os.Stderr, "failed to register: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Registered: id=%s, endpoint=%s\n", instance.ID, instance.FirstEndpoint())

	// 5. Assemble the App with BeforeStop hook for deregistration.
	//    BeforeStop runs BEFORE any server's Stop, ensuring consumers
	//    stop seeing this instance before the server actually shuts down.
	app := wind.New(
		wind.WithID(serviceID),
		wind.WithName(serviceName),
		wind.WithVersion(serviceVer),
		wind.WithServer(httpSrv),
		wind.WithStopTimeout(15*time.Second),
		wind.WithBeforeStop(func(_ context.Context) error {
			fmt.Printf("Deregistering service %q from etcd...\n", instance.Name)
			if err := reg.Deregister(context.Background(), instance); err != nil {
				fmt.Fprintf(os.Stderr, "deregister error: %v\n", err)
			} else {
				fmt.Println("Deregistered successfully.")
			}
			return nil
		}),
	)

	fmt.Println()
	fmt.Println("Service is running. Press Ctrl+C to shut down.")
	fmt.Printf("  Health:  %s/health\n", httpSrv.Endpoint())
	fmt.Println()

	if err := app.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "application exited with error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("application stopped gracefully")
}
