module github.com/tx7do/go-wind-plugins/security/authz/casbin

go 1.26.3

replace github.com/tx7do/go-wind-plugins/security/authz => ../

require (
	github.com/casbin/casbin/v2 v2.135.0
	github.com/stretchr/testify v1.11.1
	github.com/tx7do/go-wind v0.0.1
	github.com/tx7do/go-wind-plugins/security/authz v0.0.0-00010101000000-000000000000
)

require (
	github.com/bmatcuk/doublestar/v4 v4.10.0 // indirect
	github.com/casbin/govaluate v1.10.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.11.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260427160629-7cedc36a6bc4 // indirect
	google.golang.org/grpc v1.80.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
