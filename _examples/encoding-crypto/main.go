// Package main demonstrates automatic content negotiation and transparent
// body encryption/decryption using the codec and crypto middleware.
//
// The pipeline is:
//
//	Request:  client → crypto.Middleware (decrypt) → codec.Middleware (store codec)
//	          → handler (codec.ReadBody to unmarshal plaintext)
//	Response: handler → codec.Respond (marshal) → crypto.Middleware (encrypt)
//	          → client
//
// Run:
//
//	go run ./examples/encoding-crypto
//
// Test (plaintext, no encryption):
//
//	curl -H "Content-Type: application/json" -d '{"name":"alice"}' http://localhost:8080/secure
//
// Test (encrypted — requires a client that AES-encrypts the body with the
// same 16-byte key):
//
//	See the test file in transport/http/middleware/crypto for the encryption flow.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	utilsCrypto "github.com/tx7do/go-utils/crypto"
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // side-effect: register JSON codec
	httpServer "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/driver/std"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/codec"
	httpCrypto "github.com/tx7do/go-wind-plugins/transport/http/middleware/crypto"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/recovery"
)

type secureRequest struct {
	Name string `json:"name"`
}

type secureResponse struct {
	Message string `json:"message"`
}

func main() {
	// 16-byte AES-128 key. In production, load from a secret manager.
	aesKey := []byte("1234567890abcdef")
	cipher := utilsCrypto.NewAESCipher(aesKey, nil)

	srv := httpServer.NewServer(":8080", httpServer.WithDriver(std.NewDriver()))

	// Middleware order: crypto BEFORE codec.
	//   crypto decrypts the body first, then codec parses the plaintext.
	srv.Use(
		recovery.Middleware(),
		httpCrypto.Middleware(cipher),
		codec.Middleware(),
	)

	srv.POST("/secure", func(w http.ResponseWriter, r *http.Request) {
		var req secureRequest
		if err := codec.ReadBody(r, &req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		codec.Respond(w, r, http.StatusOK, &secureResponse{
			Message: "decrypted OK, hello " + req.Name,
		})
	})

	// Graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("HTTP server (encrypted) listening on %s\n", srv.Endpoint())
	fmt.Println("AES-128 key: 1234567890abcdef")
	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("server stopped")
}
