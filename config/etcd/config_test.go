package etcd

import (
	"context"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
)

const testKey = "/go-wind/test/config"

func createTestEtcdClient() (*clientv3.Client, error) {
	return clientv3.New(clientv3.Config{
		Endpoints:   []string{"127.0.0.1:2379"},
		DialTimeout: time.Second,
		DialOptions: []grpc.DialOption{grpc.WithBlock()},
	})
}

func TestConfig(t *testing.T) {
	client, err := createTestEtcdClient()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = client.Close()
	}()
	if _, err = client.Put(context.Background(), testKey, "test config"); err != nil {
		t.Fatal(err)
	}

	src, err := New(client, WithPath(testKey))
	if err != nil {
		t.Fatal(err)
	}

	data, err := src.Load(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "test config" {
		t.Fatalf("config error: got %q, want %q", string(data), "test config")
	}

	ch, err := src.WatchValue(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}

	if _, err = client.Put(context.Background(), testKey, "new config"); err != nil {
		t.Error(err)
	}

	val := <-ch
	if string(val) != "new config" {
		t.Fatalf("watch error: got %q, want %q", string(val), "new config")
	}

	if _, err = client.Delete(context.Background(), testKey); err != nil {
		t.Error(err)
	}
}

func TestEtcdWithPath(t *testing.T) {
	tests := []struct {
		name   string
		fields string
		want   string
	}{
		{
			name:   "default",
			fields: testKey,
			want:   testKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &options{
				ctx: context.Background(),
			}

			got := WithPath(tt.fields)
			got(o)

			if o.path != tt.want {
				t.Errorf("WithPath(tt.fields) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEtcdWithPrefix(t *testing.T) {
	tests := []struct {
		name   string
		fields bool
		want   bool
	}{
		{
			name:   "default",
			fields: false,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &options{
				ctx: context.Background(),
			}

			got := WithPrefix(tt.fields)
			got(o)

			if o.prefix != tt.want {
				t.Errorf("WithPrefix(tt.fields) = %v, want %v", got, tt.want)
			}
		})
	}
}
