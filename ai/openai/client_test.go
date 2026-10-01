package openai

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sashabaranov/go-openai"
)

// ---------------------------------------------------------------------------
// NewClient configuration validation
// ---------------------------------------------------------------------------

func TestNewClient_NilConfig(t *testing.T) {
	c, err := NewClient(nil)
	if err == nil {
		t.Error("NewClient(nil) should return an error")
	}
	if c != nil {
		t.Error("NewClient(nil) should return a nil client")
	}
}

func TestNewClient_UnsupportedType(t *testing.T) {
	c, err := NewClient(&Config{Type: ModelType(99)})
	if err == nil {
		t.Error("NewClient with unsupported type should return an error")
	}
	if !strings.Contains(err.Error(), "unsupported ai model type") {
		t.Errorf("error = %q, want it to mention unsupported ai model type", err.Error())
	}
	if c != nil {
		t.Error("NewClient with unsupported type should return a nil client")
	}
}

func TestNewClient_CloudWithoutCloudConfig(t *testing.T) {
	c, err := NewClient(&Config{Type: ModelTypeCloud})
	if err == nil {
		t.Error("cloud client without Cloud config should return an error")
	}
	if !strings.Contains(err.Error(), "cloud config is nil") {
		t.Errorf("error = %q, want %q", err.Error(), "cloud config is nil")
	}
	if c != nil {
		t.Error("cloud client without Cloud config should return a nil client")
	}
}

func TestNewClient_LocalWithoutLocalConfig(t *testing.T) {
	c, err := NewClient(&Config{Type: ModelTypeLocal})
	if err == nil {
		t.Error("local client without Local config should return an error")
	}
	if !strings.Contains(err.Error(), "local config is nil") {
		t.Errorf("error = %q, want %q", err.Error(), "local config is nil")
	}
	if c != nil {
		t.Error("local client without Local config should return a nil client")
	}
}

func TestNewClient_Cloud(t *testing.T) {
	c, err := NewClient(&Config{
		Type: ModelTypeCloud,
		Cloud: &CloudConfig{
			ApiKey:       "test-key",
			BaseUrl:      "https://api.example.com/v1",
			Organization: "org-123",
		},
	})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	if c == nil {
		t.Fatal("NewClient returned nil client")
	}
}

func TestNewClient_Local(t *testing.T) {
	// Constructing the client must not dial the local endpoint.
	c, err := NewClient(&Config{
		Type:  ModelTypeLocal,
		Local: &LocalConfig{Host: "127.0.0.1", Port: 11434},
	})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	if c == nil {
		t.Fatal("NewClient returned nil client")
	}
}

// ---------------------------------------------------------------------------
// Options and setHTTPClient helpers
// ---------------------------------------------------------------------------

func TestApplyOptions_Empty(t *testing.T) {
	o := applyOptions(nil)
	if o == nil {
		t.Fatal("applyOptions should never return nil")
	}
	if o.httpClient != nil {
		t.Error("default options should have a nil httpClient")
	}
}

func TestApplyOptions_WithHTTPClient(t *testing.T) {
	custom := &http.Client{Timeout: 42 * time.Second}
	o := applyOptions([]Option{WithHTTPClient(custom)})
	if o.httpClient != custom {
		t.Error("WithHTTPClient should store the provided client")
	}
}

func TestSetHTTPClient_UsesCustomClient(t *testing.T) {
	custom := &http.Client{Timeout: 42 * time.Second}
	o := applyOptions([]Option{WithHTTPClient(custom)})

	cfg := openai.DefaultConfig("key")
	setHTTPClient(&Config{}, o, &cfg)

	if cfg.HTTPClient != httpDoer(custom) {
		t.Error("setHTTPClient should use the custom HTTP client")
	}
}

func TestSetHTTPClient_DefaultTimeout(t *testing.T) {
	o := applyOptions(nil)

	cfg := openai.DefaultConfig("key")
	setHTTPClient(&Config{}, o, &cfg)

	hc, ok := cfg.HTTPClient.(*http.Client)
	if !ok {
		t.Fatalf("HTTPClient type = %T, want *http.Client", cfg.HTTPClient)
	}
	if hc.Timeout != 30*time.Second {
		t.Errorf("default timeout = %v, want 30s", hc.Timeout)
	}
}

func TestSetHTTPClient_ConfigTimeout(t *testing.T) {
	o := applyOptions(nil)

	cfg := openai.DefaultConfig("key")
	setHTTPClient(&Config{TimeoutSeconds: 5}, o, &cfg)

	hc, ok := cfg.HTTPClient.(*http.Client)
	if !ok {
		t.Fatalf("HTTPClient type = %T, want *http.Client", cfg.HTTPClient)
	}
	if hc.Timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", hc.Timeout)
	}
}

// httpDoer widens *http.Client to the HTTPDoer interface used by go-openai.
func httpDoer(c *http.Client) interface {
	Do(req *http.Request) (*http.Response, error)
} {
	return c
}

// ---------------------------------------------------------------------------
// Cloud client against httptest.Server (base URL injection)
// ---------------------------------------------------------------------------

func TestCloudClient_ChatCompletion(t *testing.T) {
	var (
		gotPath   string
		gotAuth   string
		gotOrg    string
		gotModel  string
		gotPrompt string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotOrg = r.Header.Get("OpenAI-Organization")

		var req openai.ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		gotModel = req.Model
		if len(req.Messages) > 0 {
			gotPrompt = req.Messages[0].Content
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openai.ChatCompletionResponse{
			ID:      "chatcmpl-1",
			Object:  "chat.completion",
			Created: 1700000000,
			Model:   "gpt-test",
			Choices: []openai.ChatCompletionChoice{{
				Index: 0,
				Message: openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleAssistant,
					Content: "hello from test server",
				},
				FinishReason: "stop",
			}},
		})
	}))
	defer srv.Close()

	c, err := NewClient(&Config{
		Type: ModelTypeCloud,
		Cloud: &CloudConfig{
			ApiKey:       "secret-key",
			BaseUrl:      srv.URL,
			Organization: "org-abc",
		},
	})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	resp, err := c.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
		Model: "gpt-test",
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: "hi"},
		},
	})
	if err != nil {
		t.Fatalf("CreateChatCompletion returned error: %v", err)
	}

	// Request building checks.
	if gotPath != "/chat/completions" {
		t.Errorf("request path = %q, want %q", gotPath, "/chat/completions")
	}
	if gotAuth != "Bearer secret-key" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer secret-key")
	}
	if gotOrg != "org-abc" {
		t.Errorf("OpenAI-Organization header = %q, want %q", gotOrg, "org-abc")
	}
	if gotModel != "gpt-test" {
		t.Errorf("request model = %q, want %q", gotModel, "gpt-test")
	}
	if gotPrompt != "hi" {
		t.Errorf("request prompt = %q, want %q", gotPrompt, "hi")
	}

	// Response parsing checks.
	if resp.ID != "chatcmpl-1" {
		t.Errorf("resp.ID = %q, want %q", resp.ID, "chatcmpl-1")
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("len(resp.Choices) = %d, want 1", len(resp.Choices))
	}
	if resp.Choices[0].Message.Content != "hello from test server" {
		t.Errorf("choice content = %q, want %q", resp.Choices[0].Message.Content, "hello from test server")
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("finish reason = %q, want %q", resp.Choices[0].FinishReason, "stop")
	}
}

func TestCloudClient_CustomHTTPClientUsed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","model":"m","choices":[]}`))
	}))
	defer srv.Close()

	uaTransport := &recordingTransport{base: http.DefaultTransport}
	custom := &http.Client{Transport: uaTransport}

	c, err := NewClient(&Config{
		Type:  ModelTypeCloud,
		Cloud: &CloudConfig{ApiKey: "k", BaseUrl: srv.URL},
	}, WithHTTPClient(custom))
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	_, err = c.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
		Model:    "m",
		Messages: []openai.ChatCompletionMessage{{Role: openai.ChatMessageRoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("CreateChatCompletion returned error: %v", err)
	}
	if !uaTransport.used {
		t.Error("the custom HTTP client transport should have been used for the request")
	}
}

// recordingTransport records whether it served a request.
type recordingTransport struct {
	base http.RoundTripper
	used bool
}

func (rt *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.used = true
	return rt.base.RoundTrip(req)
}

func TestCloudClient_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"boom","type":"server_error"}}`, http.StatusInternalServerError)
	}))
	defer srv.Close()

	c, err := NewClient(&Config{
		Type:  ModelTypeCloud,
		Cloud: &CloudConfig{ApiKey: "k", BaseUrl: srv.URL},
	})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	_, err = c.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
		Model:    "m",
		Messages: []openai.ChatCompletionMessage{{Role: openai.ChatMessageRoleUser, Content: "hi"}},
	})
	if err == nil {
		t.Fatal("CreateChatCompletion should fail when the server returns 500")
	}
}

// ---------------------------------------------------------------------------
// Local client against httptest.Server (host/port injection)
// ---------------------------------------------------------------------------

func TestLocalClient_ChatCompletion(t *testing.T) {
	// Reserve a port on 127.0.0.1 so LocalConfig can reference it explicitly.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen returned error: %v", err)
	}
	port := lis.Addr().(*net.TCPAddr).Port

	var gotPath string
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openai.ChatCompletionResponse{
			ID:     "local-1",
			Object: "chat.completion",
			Model:  "llama3",
			Choices: []openai.ChatCompletionChoice{{
				Index:        0,
				Message:      openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: "local reply"},
				FinishReason: "stop",
			}},
		})
	}))
	srv.Listener = lis
	srv.Start()
	defer srv.Close()

	c, err := NewClient(&Config{
		Type:  ModelTypeLocal,
		Local: &LocalConfig{Host: "127.0.0.1", Port: int32(port)},
	})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	resp, err := c.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
		Model:    "llama3",
		Messages: []openai.ChatCompletionMessage{{Role: openai.ChatMessageRoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("CreateChatCompletion returned error: %v", err)
	}

	// The local client must build the /v1 prefixed base URL.
	if gotPath != "/v1/chat/completions" {
		t.Errorf("request path = %q, want %q", gotPath, "/v1/chat/completions")
	}
	if resp.ID != "local-1" {
		t.Errorf("resp.ID = %q, want %q", resp.ID, "local-1")
	}
	if len(resp.Choices) != 1 || resp.Choices[0].Message.Content != "local reply" {
		t.Errorf("unexpected choices: %+v", resp.Choices)
	}
}
