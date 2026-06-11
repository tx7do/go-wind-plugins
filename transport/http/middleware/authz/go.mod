module github.com/tx7do/go-wind-plugins/transport/http/middleware/authz

go 1.26.3

require (
	github.com/stretchr/testify v1.11.1
	github.com/tx7do/go-wind-plugins/security/authn v0.0.1
	github.com/tx7do/go-wind-plugins/security/authz v0.0.1
	github.com/tx7do/go-wind-plugins/transport/http v0.0.1
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/tx7do/go-wind v0.0.1 // indirect
	github.com/tx7do/go-wind-plugins/errors v0.0.1 // indirect
	golang.org/x/sys v0.43.0 // indirect
	google.golang.org/grpc v1.80.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/tx7do/go-wind-plugins/security/authn => ../../../../security/authn
	github.com/tx7do/go-wind-plugins/security/authz => ../../../../security/authz
	github.com/tx7do/go-wind-plugins/transport/http => ../..
)
