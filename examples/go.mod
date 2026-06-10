module github.com/tx7do/go-wind-plugins/examples

go 1.26.3

require (
	github.com/tx7do/go-utils/crypto v0.0.2
	github.com/tx7do/go-wind v0.0.1
	github.com/tx7do/go-wind-plugins/encoding/json v0.0.1
	github.com/tx7do/go-wind-plugins/encoding/xml v0.0.0
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
	google.golang.org/grpc v1.80.0
)

require (
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/tjfoc/gmsm v1.4.1 // indirect
	github.com/tx7do/go-wind-plugins/encoding v0.0.1 // indirect
	github.com/tx7do/go-wind-plugins/security/crypto v0.0.0 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260427160629-7cedc36a6bc4 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace (
	github.com/tx7do/go-wind-plugins/encoding => ../encoding
	github.com/tx7do/go-wind-plugins/encoding/json => ../encoding/json
	github.com/tx7do/go-wind-plugins/encoding/xml => ../encoding/xml
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
)
