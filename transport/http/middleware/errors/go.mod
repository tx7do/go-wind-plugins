module github.com/tx7do/go-wind-plugins/transport/http/middleware/errors

go 1.26.3

require (
	github.com/tx7do/go-wind-plugins/errors v0.0.1
	github.com/tx7do/go-wind-plugins/transport/http v0.0.1
)

require github.com/tx7do/go-wind v0.0.1 // indirect

replace (
	github.com/tx7do/go-wind-plugins/errors => ../../../../errors
	github.com/tx7do/go-wind-plugins/transport/http => ../..
)
