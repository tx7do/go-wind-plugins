package codec_test

import (
	"fmt"
	"net/http"

	windhttp "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/codec"
)

type greetRequest struct {
	Name string `json:"name" xml:"name"`
}

type greetResponse struct {
	Message string `json:"message" xml:"message"`
}

// ExampleMiddleware attaches the codec middleware, which performs automatic
// content negotiation. Handlers read request bodies and write responses
// through the package-level ReadBody/Respond helpers, and the codec selected
// from the request's Content-Type/Accept headers does the serialization.
//
// Codec implementations are registered by importing the encoding packages for
// their side effects, e.g.
// _ "github.com/tx7do/go-wind-plugins/encoding/json". A runnable end-to-end
// setup additionally needs a driver (see transport/http/driver/std).
func ExampleMiddleware() {
	srv := windhttp.NewServer(":8080")
	srv.Use(codec.Middleware())

	srv.POST("/echo", func(w http.ResponseWriter, r *http.Request) {
		var req greetRequest
		if err := codec.ReadBody(r, &req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		codec.Respond(w, r, http.StatusOK, &greetResponse{
			Message: "Hello, " + req.Name + "!",
		})
	})

	fmt.Println(srv.Endpoint())
}
