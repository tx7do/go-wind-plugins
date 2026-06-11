module github.com/tx7do/go-wind-plugins/transport/grpc/middleware/crypto

go 1.26.3

require (
	github.com/tx7do/go-utils/crypto v0.0.2
	github.com/tx7do/go-wind-plugins/security/crypto v0.0.1
	google.golang.org/grpc v1.80.0
)

require (
	github.com/tjfoc/gmsm v1.4.1 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260420184626-e10c466a9529 // indirect
)

replace github.com/tx7do/go-wind-plugins/security/crypto => ../../../../security/crypto
