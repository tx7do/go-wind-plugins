package etcd_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/config/etcd"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// ExampleNew constructs an etcd-backed configuration source. Load performs a
// one-shot read of the configured key; WatchValue delivers updated values on
// the returned channel so the application can reload configuration live.
func ExampleNew() {
	client, err := clientv3.New(clientv3.Config{
		Endpoints: []string{"localhost:2379"},
	})
	if err != nil {
		return
	}
	defer client.Close()

	src, err := etcd.New(client,
		etcd.WithPath("/myapp/config"),
	)
	if err != nil {
		return
	}

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
