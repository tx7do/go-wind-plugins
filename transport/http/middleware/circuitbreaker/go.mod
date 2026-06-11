module github.com/tx7do/go-wind-plugins/transport/http/middleware/circuitbreaker

go 1.26.3

require (
	github.com/stretchr/testify v1.11.1
	github.com/tx7do/go-wind-plugins/circuitbreaker v0.0.1
	github.com/tx7do/go-wind-plugins/transport/http v0.0.1
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/tx7do/go-wind v0.0.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/tx7do/go-wind-plugins/circuitbreaker => ../../../../circuitbreaker
	github.com/tx7do/go-wind-plugins/transport/http => ../..
)
