package http_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/config/http"
)

// ExampleNew constructs an HTTP-backed configuration source. The URL option
// names the endpoint configuration is fetched from, and header options such as
// auth tokens are attached to every request. Load performs a one-shot fetch;
// WatchValue polls the endpoint and delivers changed response bodies on the
// returned channel so configuration can be reloaded live.
func ExampleNew() {
	src, err := http.New(
		http.WithURL("http://localhost:8080/config/myapp.yaml"),
		http.WithHeader("Authorization", "Bearer placeholder-token"),
	)
	if err != nil {
		return
	}
	defer src.Close()

	raw, err := src.Load(context.Background(), "")
	if err != nil {
		return
	}
	_ = raw

	ch, err := src.WatchValue(context.Background(), "")
	if err != nil {
		return
	}
	_ = ch
}
