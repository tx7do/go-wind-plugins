package nacos

import (
	"context"
	"testing"
	"time"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

func TestConfig_Load(t *testing.T) {
	sc := []constant.ServerConfig{
		*constant.NewServerConfig("127.0.0.1", 8848),
	}

	cc := constant.ClientConfig{
		TimeoutMs:           5000,
		NotLoadCacheAtStart: true,
		LogDir:              "/tmp/nacos/log",
		CacheDir:            "/tmp/nacos/cache",
		LogLevel:            "debug",
	}

	client, err := clients.NewConfigClient(
		vo.NacosClientParam{
			ClientConfig:  &cc,
			ServerConfigs: sc,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	source := New(client, WithGroup("test"), WithDataID("test.yaml"))

	tests := []struct {
		name      string
		wantErr   bool
		wantData  string
		preFunc   func(t *testing.T)
		deferFunc func(t *testing.T)
	}{
		{
			name:     "normal",
			wantErr:  false,
			wantData: "test: test",
			preFunc: func(t *testing.T) {
				_, err = client.PublishConfig(vo.ConfigParam{DataId: "test.yaml", Group: "test", Content: "test: test"})
				if err != nil {
					t.Error(err)
				}
				time.Sleep(time.Second * 1)
			},
			deferFunc: func(t *testing.T) {
				_, dErr := client.DeleteConfig(vo.ConfigParam{DataId: "test.yaml", Group: "test"})
				if dErr != nil {
					t.Error(dErr)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.preFunc != nil {
				test.preFunc(t)
			}
			if test.deferFunc != nil {
				defer test.deferFunc(t)
			}
			data, lErr := source.Load(context.Background(), "")
			if (lErr != nil) != test.wantErr {
				t.Errorf("Load error = %v, wantErr %v", lErr, test.wantErr)
				return
			}
			if string(data) != test.wantData {
				t.Errorf("Load data = %q, want %q", string(data), test.wantData)
			}
		})
	}
}

func TestConfig_WatchValue(t *testing.T) {
	sc := []constant.ServerConfig{
		*constant.NewServerConfig("127.0.0.1", 8848),
	}

	cc := constant.ClientConfig{
		TimeoutMs:           5000,
		NotLoadCacheAtStart: true,
		LogDir:              "/tmp/nacos/log",
		CacheDir:            "/tmp/nacos/cache",
		LogLevel:            "debug",
	}

	client, err := clients.NewConfigClient(
		vo.NacosClientParam{
			ClientConfig:  &cc,
			ServerConfigs: sc,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	source := New(client, WithGroup("test"), WithDataID("test.yaml"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, wErr := source.WatchValue(ctx, "")
	if wErr != nil {
		t.Fatal(wErr)
	}

	_, pErr := client.PublishConfig(vo.ConfigParam{DataId: "test.yaml", Group: "test", Content: "test: test"})
	if pErr != nil {
		t.Fatal(pErr)
	}
	defer func() {
		_, _ = client.DeleteConfig(vo.ConfigParam{DataId: "test.yaml", Group: "test"})
	}()

	val := <-ch
	if string(val) != "test: test" {
		t.Errorf("WatchValue got %q, want %q", string(val), "test: test")
	}
}
