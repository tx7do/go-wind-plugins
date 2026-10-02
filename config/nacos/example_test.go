package nacos_test

import (
	"context"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"github.com/tx7do/go-wind-plugins/config/nacos"
)

// ExampleNew constructs a Nacos-backed configuration source. Load performs a
// one-shot read of the configured data ID, and WatchValue delivers updated
// content on the returned channel whenever Nacos publishes a new version of
// it.
func ExampleNew() {
	sc := []constant.ServerConfig{
		*constant.NewServerConfig("127.0.0.1", 8848),
	}

	cc := constant.ClientConfig{
		TimeoutMs: 5000,
	}

	client, err := clients.NewConfigClient(
		vo.NacosClientParam{
			ClientConfig:  &cc,
			ServerConfigs: sc,
		},
	)
	if err != nil {
		return
	}

	src := nacos.New(client,
		nacos.WithGroup("my-group"),
		nacos.WithDataID("my-app.yaml"),
	)

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
