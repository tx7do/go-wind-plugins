module github.com/tx7do/go-wind-plugins/security/authn/presharedkey

go 1.26.3

require (
	github.com/stretchr/testify v1.11.1
	github.com/tx7do/go-wind-plugins/security/authn v0.0.1
	google.golang.org/grpc v1.80.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/tx7do/go-wind-plugins/errors v0.0.1 // indirect
	golang.org/x/sys v0.43.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/tx7do/go-wind-plugins/security/authn => ../
