module github.com/tx7do/go-wind-plugins/security/authn

go 1.26.3

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0
	github.com/stretchr/testify v1.11.1
	github.com/tx7do/go-wind-plugins/errors v0.0.0
)

replace github.com/tx7do/go-wind-plugins/errors => ../../errors

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.11.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	google.golang.org/grpc v1.80.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
