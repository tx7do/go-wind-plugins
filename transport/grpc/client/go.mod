module github.com/tx7do/go-wind-plugins/transport/grpc/client

go 1.26.3

require google.golang.org/grpc v1.80.0

require (
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260427160629-7cedc36a6bc4 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace google.golang.org/genproto => google.golang.org/genproto v0.0.0-20260406210006-6f92a3bedf2d

replace github.com/lyft/protoc-gen-validate => github.com/envoyproxy/protoc-gen-validate v0.10.1
