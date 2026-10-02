package kubernetes_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/config/kubernetes"
)

// ExampleNew constructs a Kubernetes ConfigMap-backed configuration source.
// Keys name a configmap as "namespace/name" or as a bare name resolved
// against the configured namespace. Load performs a one-shot read, and
// WatchValue delivers updated configmap content on the returned channel so
// in-cluster services can reload configuration without a restart.
func ExampleNew() {
	src := kubernetes.New(
		kubernetes.WithNamespace("my-namespace"),
	)

	raw, err := src.Load(context.Background(), "my-app-config")
	if err != nil {
		return
	}
	_ = raw

	ch, err := src.WatchValue(context.Background(), "my-app-config")
	if err != nil {
		return
	}
	_ = ch
}
