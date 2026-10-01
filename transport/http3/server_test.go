package http3

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"

	"github.com/stretchr/testify/assert"
)

// skipWithoutIntegration skips tests that need a running HTTP/3 server.
func skipWithoutIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("KRATOS_IT") == "" {
		t.Skip("skipping integration test: set KRATOS_IT to enable")
	}
}

func HygrothermographHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("HygrothermographHandler [%s] [%s] [%s]\n", r.Proto, r.Method, r.RequestURI)

	if r.Method == "POST" {
		var in map[string]any
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			fmt.Printf("decode error: %s\n", err.Error())
		}
		fmt.Printf("Payload: %v\n", in)
	}

	w.Header().Set("Content-Type", "application/json")
	var out = map[string]string{
		"Humidity":    strconv.FormatInt(int64(rand.Intn(100)), 10),
		"Temperature": strconv.FormatInt(int64(rand.Intn(100)), 10),
	}
	_ = json.NewEncoder(w).Encode(&out)
}

func TestServer(t *testing.T) {
	srv := NewServer(
		WithAddress(":8800"),
	)

	srv.HandleFunc("/hygrothermograph", HygrothermographHandler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := srv.Start(ctx); err != nil {
			panic(err)
		}
	}()

	defer func() {
		cancel()
		if err := srv.Stop(context.Background()); err != nil {
			t.Errorf("expected nil got %v", err)
		}
	}()
}

func TestClient(t *testing.T) {
	skipWithoutIntegration(t)

	// Start a dedicated server for this test. TestServer's lifecycle (its
	// deferred cancel+Stop) is scoped to that test, so it cannot serve
	// requests here — this test owns its server from start to teardown.
	srv := NewServer(
		WithAddress(":8800"),
	)
	srv.HandleFunc("/hygrothermograph", HygrothermographHandler)

	srvCtx, srvCancel := context.WithCancel(context.Background())
	srvDone := make(chan error, 1)
	go func() {
		srvDone <- srv.Start(srvCtx)
	}()
	defer func() {
		srvCancel()
		if err := srv.Stop(context.Background()); err != nil {
			t.Errorf("expected nil got %v", err)
		}
		select {
		case <-srvDone:
		case <-time.After(3 * time.Second):
			t.Error("Start did not return within 3s after Stop")
		}
	}()

	transport := &http3.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		QUICConfig:      &quic.Config{},
	}
	cli := &http.Client{Transport: transport}
	defer transport.Close()

	// Wait until the server accepts requests before asserting on them.
	deadline := time.Now().Add(10 * time.Second)
	var lastErr error
	for {
		probe, err := cli.Get("https://127.0.0.1:8800/hygrothermograph")
		if err == nil {
			_ = probe.Body.Close()
			break
		}
		lastErr = err
		if time.Now().After(deadline) {
			t.Fatalf("server not reachable within 10s: %v", lastErr)
		}
		time.Sleep(100 * time.Millisecond)
	}

	req := map[string]string{
		"Humidity":    strconv.FormatInt(int64(rand.Intn(100)), 10),
		"Temperature": strconv.FormatInt(int64(rand.Intn(100)), 10),
	}

	// GET
	resp, err := cli.Get("https://127.0.0.1:8800/hygrothermograph")
	assert.Nil(t, err)
	assert.NotNil(t, resp)
	if resp != nil {
		defer resp.Body.Close()
		var result map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&result)
		t.Logf("GET response: %v", result)
	}

	// POST
	body, _ := json.Marshal(req)
	resp, err = cli.Post("https://127.0.0.1:8800/hygrothermograph", "application/json", bytes.NewReader(body))
	assert.Nil(t, err)
	assert.NotNil(t, resp)
	if resp != nil {
		defer resp.Body.Close()
		var result map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&result)
		t.Logf("POST response: %v", result)
	}
}
