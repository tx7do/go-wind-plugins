package eino

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	einoOpenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// ---------------------------------------------------------------------------
// Options and applyConfigModifier
// ---------------------------------------------------------------------------

func TestApplyOptions_Empty(t *testing.T) {
	o := applyOptions(nil)
	if o == nil {
		t.Fatal("applyOptions should never return nil")
	}
	if o.configModifier != nil {
		t.Error("default options should have a nil configModifier")
	}
}

func TestApplyOptions_WithConfigModifier(t *testing.T) {
	mod := func(cfg *einoOpenai.ChatModelConfig) {}
	o := applyOptions([]Option{WithConfigModifier(mod)})
	if o == nil {
		t.Fatal("applyOptions should never return nil")
	}
	if o.configModifier == nil {
		t.Error("WithConfigModifier should store the provided modifier")
	}
}

func TestApplyConfigModifier_NilModifier(t *testing.T) {
	cfg := &einoOpenai.ChatModelConfig{APIKey: "k", Model: "m"}

	got := applyConfigModifier(cfg, &options{})

	if got != cfg {
		t.Error("applyConfigModifier should return the same config instance")
	}
	if cfg.APIKey != "k" || cfg.Model != "m" {
		t.Error("applyConfigModifier with a nil modifier should not change the config")
	}
}

func TestApplyConfigModifier_MutatesConfig(t *testing.T) {
	temp := float32(0.7)
	maxTokens := 4096
	cfg := &einoOpenai.ChatModelConfig{APIKey: "k", Model: "m"}

	o := applyOptions([]Option{WithConfigModifier(func(c *einoOpenai.ChatModelConfig) {
		c.Temperature = &temp
		c.MaxTokens = &maxTokens
	})})

	got := applyConfigModifier(cfg, o)

	if got.Temperature != &temp {
		t.Error("config modifier should set Temperature")
	}
	if got.MaxTokens != &maxTokens {
		t.Error("config modifier should set MaxTokens")
	}
}

// ---------------------------------------------------------------------------
// NewChatModel configuration validation
// ---------------------------------------------------------------------------

func TestNewChatModel_NilConfig(t *testing.T) {
	m, err := NewChatModel(context.Background(), nil)
	if err == nil {
		t.Error("NewChatModel(nil) should return an error")
	}
	if !strings.Contains(err.Error(), "config is nil") {
		t.Errorf("error = %q, want it to mention nil config", err.Error())
	}
	if m != nil {
		t.Error("NewChatModel(nil) should return a nil model")
	}
}

func TestNewChatModel_UnsupportedType(t *testing.T) {
	m, err := NewChatModel(context.Background(), &Config{Type: ModelType(99)})
	if err == nil {
		t.Error("NewChatModel with unsupported type should return an error")
	}
	if !strings.Contains(err.Error(), "unsupported ai model type") {
		t.Errorf("error = %q, want it to mention unsupported ai model type", err.Error())
	}
	if m != nil {
		t.Error("NewChatModel with unsupported type should return a nil model")
	}
}

func TestNewChatModel_CloudWithoutCloudConfig(t *testing.T) {
	m, err := NewChatModel(context.Background(), &Config{Type: ModelTypeCloud})
	if err == nil {
		t.Error("cloud model without Cloud config should return an error")
	}
	if !strings.Contains(err.Error(), "cloud config is nil") {
		t.Errorf("error = %q, want %q", err.Error(), "cloud config is nil")
	}
	if m != nil {
		t.Error("cloud model without Cloud config should return a nil model")
	}
}

func TestNewChatModel_LocalWithoutLocalConfig(t *testing.T) {
	m, err := NewChatModel(context.Background(), &Config{Type: ModelTypeLocal})
	if err == nil {
		t.Error("local model without Local config should return an error")
	}
	if !strings.Contains(err.Error(), "local config is nil") {
		t.Errorf("error = %q, want %q", err.Error(), "local config is nil")
	}
	if m != nil {
		t.Error("local model without Local config should return a nil model")
	}
}

// ---------------------------------------------------------------------------
// NewChatModel construction (must not dial)
// ---------------------------------------------------------------------------

func TestNewChatModel_CloudConstruction(t *testing.T) {
	m, err := NewChatModel(context.Background(), &Config{
		Type:      ModelTypeCloud,
		ModelName: "gpt-test",
		Cloud: &CloudConfig{
			ApiKey:  "test-key",
			BaseUrl: "https://api.example.com/v1",
		},
		TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("NewChatModel returned error: %v", err)
	}
	var cm model.ChatModel = m
	_ = cm
}

func TestNewChatModel_LocalConstruction(t *testing.T) {
	// Constructing the model must not dial the local endpoint.
	m, err := NewChatModel(context.Background(), &Config{
		Type:      ModelTypeLocal,
		ModelName: "llama3",
		Local:     &LocalConfig{Host: "127.0.0.1", Port: 11434},
	})
	if err != nil {
		t.Fatalf("NewChatModel returned error: %v", err)
	}
	if m == nil {
		t.Fatal("NewChatModel returned nil model")
	}
}

// ---------------------------------------------------------------------------
// Cloud model against httptest.Server (base URL injection)
// ---------------------------------------------------------------------------

// chatRequest is the subset of the OpenAI chat completion request we assert on.
type chatRequest struct {
	Model       string   `json:"model"`
	Temperature *float64 `json:"temperature"`
	Messages    []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func newTestChatServer(t *testing.T, status int, body string, got *chatRequest, gotAuth *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotAuth != nil {
			*gotAuth = r.Header.Get("Authorization")
		}
		if got != nil {
			data, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			*got = chatRequest{}
			if err := json.Unmarshal(data, got); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

const cloudChatResponse = `{"id":"chatcmpl-1","object":"chat.completion","created":1700000000,"model":"gpt-test","choices":[{"index":0,"message":{"role":"assistant","content":"hello from test server"},"finish_reason":"stop"}]}`

func TestNewChatModel_CloudGenerate(t *testing.T) {
	var (
		gotReq  chatRequest
		gotAuth string
	)
	srv := newTestChatServer(t, http.StatusOK, cloudChatResponse, &gotReq, &gotAuth)
	defer srv.Close()

	m, err := NewChatModel(context.Background(), &Config{
		Type:      ModelTypeCloud,
		ModelName: "gpt-test",
		Cloud: &CloudConfig{
			ApiKey:  "secret-key",
			BaseUrl: srv.URL,
		},
	}, WithConfigModifier(func(cfg *einoOpenai.ChatModelConfig) {
		temp := float32(0.7)
		cfg.Temperature = &temp
	}))
	if err != nil {
		t.Fatalf("NewChatModel returned error: %v", err)
	}

	resp, err := m.Generate(context.Background(), []*schema.Message{
		{Role: schema.User, Content: "hi"},
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// Request building checks.
	if gotAuth != "Bearer secret-key" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer secret-key")
	}
	if gotReq.Model != "gpt-test" {
		t.Errorf("request model = %q, want %q", gotReq.Model, "gpt-test")
	}
	if len(gotReq.Messages) != 1 {
		t.Fatalf("len(request messages) = %d, want 1", len(gotReq.Messages))
	}
	if gotReq.Messages[0].Role != "user" || gotReq.Messages[0].Content != "hi" {
		t.Errorf("request message = %+v, want user/hi", gotReq.Messages[0])
	}
	if gotReq.Temperature == nil {
		t.Error("WithConfigModifier should have added temperature to the request")
	} else if math.Abs(*gotReq.Temperature-0.7) > 1e-4 {
		t.Errorf("request temperature = %v, want ~0.7", *gotReq.Temperature)
	}

	// Response parsing checks.
	if resp == nil {
		t.Fatal("Generate returned nil message")
	}
	if resp.Role != schema.Assistant {
		t.Errorf("resp.Role = %q, want %q", resp.Role, schema.Assistant)
	}
	if resp.Content != "hello from test server" {
		t.Errorf("resp.Content = %q, want %q", resp.Content, "hello from test server")
	}
}

func TestNewChatModel_CloudServerError(t *testing.T) {
	srv := newTestChatServer(t, http.StatusInternalServerError, `{"error":{"message":"boom","type":"server_error"}}`, nil, nil)
	defer srv.Close()

	m, err := NewChatModel(context.Background(), &Config{
		Type:      ModelTypeCloud,
		ModelName: "gpt-test",
		Cloud:     &CloudConfig{ApiKey: "k", BaseUrl: srv.URL},
	})
	if err != nil {
		t.Fatalf("NewChatModel returned error: %v", err)
	}

	_, err = m.Generate(context.Background(), []*schema.Message{
		{Role: schema.User, Content: "hi"},
	})
	if err == nil {
		t.Fatal("Generate should fail when the server returns 500")
	}
}

func TestNewChatModel_CloudEmptyChoices(t *testing.T) {
	srv := newTestChatServer(t, http.StatusOK, `{"id":"x","object":"chat.completion","model":"m","choices":[]}`, nil, nil)
	defer srv.Close()

	m, err := NewChatModel(context.Background(), &Config{
		Type:      ModelTypeCloud,
		ModelName: "gpt-test",
		Cloud:     &CloudConfig{ApiKey: "k", BaseUrl: srv.URL},
	})
	if err != nil {
		t.Fatalf("NewChatModel returned error: %v", err)
	}

	_, err = m.Generate(context.Background(), []*schema.Message{
		{Role: schema.User, Content: "hi"},
	})
	if err == nil {
		t.Fatal("Generate should fail when the response has no choices")
	}
	if !strings.Contains(err.Error(), "empty choices") {
		t.Errorf("error = %q, want it to mention empty choices", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Local (Ollama) model against httptest.Server (host/port injection)
// ---------------------------------------------------------------------------

func TestNewChatModel_LocalGenerate(t *testing.T) {
	// Reserve a port on 127.0.0.1 so LocalConfig can reference it explicitly.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen returned error: %v", err)
	}
	port := lis.Addr().(*net.TCPAddr).Port

	var (
		gotPath string
		gotAuth string
		gotReq  chatRequest
	)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(cloudChatResponse))
	}))
	srv.Listener = lis
	srv.Start()
	defer srv.Close()

	m, err := NewChatModel(context.Background(), &Config{
		Type:           ModelTypeLocal,
		ModelName:      "llama3",
		TimeoutSeconds: 3,
		Local:          &LocalConfig{Host: "127.0.0.1", Port: int32(port)},
	})
	if err != nil {
		t.Fatalf("NewChatModel returned error: %v", err)
	}

	resp, err := m.Generate(context.Background(), []*schema.Message{
		{Role: schema.User, Content: "hi"},
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// The local client must build the /v1 prefixed base URL.
	if gotPath != "/v1/chat/completions" {
		t.Errorf("request path = %q, want %q", gotPath, "/v1/chat/completions")
	}
	if gotAuth != "Bearer ollama" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer ollama")
	}
	if gotReq.Model != "llama3" {
		t.Errorf("request model = %q, want %q", gotReq.Model, "llama3")
	}
	if resp == nil || resp.Content != "hello from test server" {
		t.Errorf("resp = %+v, want content %q", resp, "hello from test server")
	}
}

// ---------------------------------------------------------------------------
// Misc
// ---------------------------------------------------------------------------

func TestModelTypeConstants(t *testing.T) {
	if ModelTypeLocal != 1 {
		t.Errorf("ModelTypeLocal = %d, want 1", ModelTypeLocal)
	}
	if ModelTypeCloud != 2 {
		t.Errorf("ModelTypeCloud = %d, want 2", ModelTypeCloud)
	}
}
