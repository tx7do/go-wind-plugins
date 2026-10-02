package zookeeper_test

import (
	"context"
	"time"

	"github.com/go-zookeeper/zk"
	"github.com/tx7do/go-wind-plugins/config/zookeeper"
)

// ExampleNew constructs a ZooKeeper-backed configuration source. Load performs
// a one-shot read of the znode at the configured path; WatchValue delivers
// updated znode content on the returned channel so the application can reload
// configuration live.
func ExampleNew() {
	conn, _, err := zk.Connect([]string{"localhost:2181"}, time.Second)
	if err != nil {
		return
	}
	defer conn.Close()

	src, err := zookeeper.New(conn,
		zookeeper.WithPath("/myapp/config"),
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
