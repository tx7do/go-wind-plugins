module github.com/tx7do/go-wind-plugins/examples

go 1.26.3

require (
	github.com/tx7do/go-utils/crypto v0.0.2
	github.com/tx7do/go-wind v0.0.1
	github.com/tx7do/go-wind-plugins/encoding/json v0.0.1
	github.com/tx7do/go-wind-plugins/encoding/xml v0.0.0
	github.com/tx7do/go-wind-plugins/registry/etcd v0.0.0
	github.com/tx7do/go-wind-plugins/transport/grpc/middleware/logging v0.0.0
	github.com/tx7do/go-wind-plugins/transport/grpc/middleware/recovery v0.0.0
	github.com/tx7do/go-wind-plugins/transport/grpc/server v0.0.0
	github.com/tx7do/go-wind-plugins/transport/http v0.0.0
	github.com/tx7do/go-wind-plugins/transport/http/driver/std v0.0.0
	github.com/tx7do/go-wind-plugins/transport/http/middleware/codec v0.0.0
	github.com/tx7do/go-wind-plugins/transport/http/middleware/crypto v0.0.0
	github.com/tx7do/go-wind-plugins/transport/http/middleware/logging v0.0.0
	github.com/tx7do/go-wind-plugins/transport/http/middleware/recovery v0.0.0
	github.com/tx7do/go-wind-plugins/transport/http/middleware/requestid v0.0.0
	github.com/tx7do/go-wind-plugins/transport/sse v0.0.0
	github.com/tx7do/go-wind-plugins/transport/websocket v0.0.0
	go.etcd.io/etcd/client/v3 v3.6.10
	google.golang.org/grpc v1.80.0
)

require (
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd/v22 v22.7.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.29.0 // indirect
	github.com/tjfoc/gmsm v1.4.1 // indirect
	github.com/tx7do/go-wind-plugins/encoding v0.0.1 // indirect
	github.com/tx7do/go-wind-plugins/registry v0.0.0-00010101000000-000000000000 // indirect
	github.com/tx7do/go-wind-plugins/security/crypto v0.0.0 // indirect
	go.etcd.io/etcd/api/v3 v3.6.10 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.6.10 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260427160629-7cedc36a6bc4 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260427160629-7cedc36a6bc4 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace (

	github.com/tx7do/go-wind-plugins/encoding => ../encoding
	github.com/tx7do/go-wind-plugins/encoding/json => ../encoding/json
	github.com/tx7do/go-wind-plugins/encoding/xml => ../encoding/xml
	github.com/tx7do/go-wind-plugins/registry => ../registry
	github.com/tx7do/go-wind-plugins/registry/etcd => ../registry/etcd
	github.com/tx7do/go-wind-plugins/security/crypto => ../security/crypto
	github.com/tx7do/go-wind-plugins/transport/grpc/middleware/logging => ../transport/grpc/middleware/logging
	github.com/tx7do/go-wind-plugins/transport/grpc/middleware/recovery => ../transport/grpc/middleware/recovery
	github.com/tx7do/go-wind-plugins/transport/grpc/server => ../transport/grpc/server
	github.com/tx7do/go-wind-plugins/transport/http => ../transport/http
	github.com/tx7do/go-wind-plugins/transport/http/driver/std => ../transport/http/driver/std
	github.com/tx7do/go-wind-plugins/transport/http/middleware/codec => ../transport/http/middleware/codec
	github.com/tx7do/go-wind-plugins/transport/http/middleware/crypto => ../transport/http/middleware/crypto
	github.com/tx7do/go-wind-plugins/transport/http/middleware/logging => ../transport/http/middleware/logging
	github.com/tx7do/go-wind-plugins/transport/http/middleware/recovery => ../transport/http/middleware/recovery
	github.com/tx7do/go-wind-plugins/transport/http/middleware/requestid => ../transport/http/middleware/requestid
	github.com/tx7do/go-wind-plugins/transport/sse => ../transport/sse
	github.com/tx7do/go-wind-plugins/transport/websocket => ../transport/websocket
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20260406210006-6f92a3bedf2d
)
