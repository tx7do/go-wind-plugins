module github.com/tx7do/go-wind-plugins/registry/eureka

go 1.26.3

require (
	github.com/tx7do/go-wind v0.0.0-20260609092115-0a5df91d8c74
	github.com/tx7do/go-wind-plugins/registry v0.0.0-00010101000000-000000000000
)

require golang.org/x/sync v0.20.0 // indirect

replace github.com/tx7do/go-wind-plugins/registry => ../
