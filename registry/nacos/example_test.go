package nacos_test

import (
	"context"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"github.com/tx7do/go-wind"
	"github.com/tx7do/go-wind-plugins/registry/nacos"
)

// ExampleNew constructs a nacos-backed service registrar. Instances are
// registered on startup — before the server starts accepting traffic, so
// consumers can discover it immediately — and deregistered in the
// application's BeforeStop hook, which runs before any server shuts down so
// consumers stop seeing the instance before it disappears.
func ExampleNew() {
	client, err := clients.NewNamingClient(vo.NacosClientParam{
		ServerConfigs: []constant.ServerConfig{
			{IpAddr: "127.0.0.1", Port: 8848},
		},
		ClientConfig: &constant.ClientConfig{
			NamespaceId: "public",
		},
	})
	if err != nil {
		return
	}

	reg := nacos.New(client,
		nacos.WithCluster("DEFAULT"),
		nacos.WithGroup("DEFAULT_GROUP"),
	)

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
