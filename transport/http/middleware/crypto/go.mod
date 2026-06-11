module github.com/tx7do/go-wind-plugins/transport/http/middleware/crypto

go 1.26.3

require (
	github.com/tx7do/go-utils/crypto v0.0.2
	github.com/tx7do/go-wind-plugins/security/crypto v0.0.1
	github.com/tx7do/go-wind-plugins/transport/http v0.0.1
)

require (
	github.com/tjfoc/gmsm v1.4.1 // indirect
	github.com/tx7do/go-wind v0.0.1 // indirect
)

replace (
	github.com/tx7do/go-wind-plugins/security/crypto => ../../../../security/crypto
	github.com/tx7do/go-wind-plugins/transport/http => ../..
)
