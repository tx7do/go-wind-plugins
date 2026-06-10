module github.com/tx7do/go-wind-plugins/transport/http/middleware/codec

go 1.26.3

require (
	github.com/tx7do/go-wind-plugins/encoding v0.0.1
	github.com/tx7do/go-wind-plugins/encoding/json v0.0.0
	github.com/tx7do/go-wind-plugins/encoding/xml v0.0.0
	github.com/tx7do/go-wind-plugins/transport/http v0.0.0
)

require github.com/tx7do/go-wind v0.0.1 // indirect

replace (
	github.com/tx7do/go-wind-plugins/encoding => ../../../../encoding
	github.com/tx7do/go-wind-plugins/encoding/json => ../../../../encoding/json
	github.com/tx7do/go-wind-plugins/encoding/xml => ../../../../encoding/xml
	github.com/tx7do/go-wind-plugins/transport/http => ../..
)
