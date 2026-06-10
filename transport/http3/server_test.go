package http3

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"testing"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"

	"github.com/stretchr/testify/assert"
)

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
	transport := &http3.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		QUICConfig:      &quic.Config{},
	}
	cli := &http.Client{Transport: transport}
	defer transport.Close()

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
