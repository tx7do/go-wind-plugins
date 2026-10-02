package kubernetes_test

import (
	"context"

	"github.com/tx7do/go-wind"
	"github.com/tx7do/go-wind-plugins/registry/kubernetes"
	kubernetesclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// ExampleNew constructs a Kubernetes-backed service registrar from the
// in-cluster client set, scoped to the namespace of the pod the application
// runs in. Instances are registered on startup — before the server starts
// accepting traffic, so consumers can discover it immediately — and
// deregistered in the application's BeforeStop hook, which runs before any
// server shuts down so consumers stop seeing the instance before it
// disappears.
func ExampleNew() {
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return
	}
	clientSet, err := kubernetesclient.NewForConfig(restConfig)
	if err != nil {
		return
	}

	reg := kubernetes.New(clientSet, kubernetes.GetNamespace())

	instance := &wind.Instance{
		ID:        "registry-demo-001",
		Name:      "demo-service",
		Version:   "1.0.0",
		Endpoints: []string{"http://localhost:8080"},
		Metadata:  map[string]string{"protocol": "http"},
	}

	_ = reg.Register(context.Background(), instance)
	_ = reg.Deregister(context.Background(), instance)
}
