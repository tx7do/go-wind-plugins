module github.com/tx7do/go-wind-plugins/config/etcd

go 1.26.3

require (
	github.com/tx7do/go-wind-plugins/config v0.0.0-00010101000000-000000000000
	go.etcd.io/etcd/client/v3 v3.6.10
	google.golang.org/grpc v1.80.0
)

require (
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd/v22 v22.7.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.29.0 // indirect
	go.etcd.io/etcd/api/v3 v3.6.10 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.6.10 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260427160629-7cedc36a6bc4 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260427160629-7cedc36a6bc4 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace google.golang.org/genproto => google.golang.org/genproto v0.0.0-20260406210006-6f92a3bedf2d

replace github.com/lyft/protoc-gen-validate => github.com/envoyproxy/protoc-gen-validate v0.10.1

replace github.com/tx7do/go-wind-plugins/config => ../
