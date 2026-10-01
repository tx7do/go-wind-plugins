package langchaingo

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tmc/langchaingo/llms"
	lcOllama "github.com/tmc/langchaingo/llms/ollama"
	lcOpenai "github.com/tmc/langchaingo/llms/openai"
)

// ---------------------------------------------------------------------------
// NewModel configuration validation
// ---------------------------------------------------------------------------

func TestNewModel_NilConfig(t *testing.T) {
	m, err := NewModel(nil)
	if err == nil {
		t.Error("NewModel(nil) should return an error")
	}
	if !strings.Contains(err.Error(), "config is nil") {
		t.Errorf("error = %q, want it to mention nil config", err.Error())
	}
	if m != nil {
		t.Error("NewModel(nil) should return a nil model")
	}
}

func TestNewModel_UnsupportedType(t *testing.T) {
	m, err := NewModel(&Config{Type: ModelType(99)})
	if err == nil {
		t.Error("NewModel with unsupported type should return an error")
	}
	if !strings.Contains(err.Error(), "unsupported ai model type") {
		t.Errorf("error = %q, want it to mention unsupported ai model type", err.Error())
	}
	if m != nil {
		t.Error("NewModel with unsupported type should return a nil model")
	}
}

func TestNewModel_CloudWithoutCloudConfig(t *testing.T) {
	m, err := NewModel(&Config{Type: ModelTypeCloud})
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

func TestNewModel_LocalWithoutLocalConfig(t *testing.T) {
	m, err := NewModel(&Config{Type: ModelTypeLocal})
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

func TestNewModel_CloudMissingToken(t *testing.T) {
	// Force an empty token regardless of the ambient environment.
	t.Setenv("OPENAI_API_KEY", "")

	_, err := NewModel(&Config{
		Type:  ModelTypeCloud,
		Cloud: &CloudConfig{ApiKey: ""},
	})
	if err == nil {
		t.Fatal("cloud model without an API key should return an error")
	}
	if !errors.Is(err, lcOpenai.ErrMissingToken) {
		t.Errorf("error = %q, want ErrMissingToken", err.Error())
	}
}

// ---------------------------------------------------------------------------
// NewModel construction (must not dial)
// ---------------------------------------------------------------------------

func TestNewModel_CloudConstruction(t *testing.T) {
	m, err := NewModel(&Config{
		Type:      ModelTypeCloud,
		ModelName: "gpt-test",
		Cloud: &CloudConfig{
			ApiKey:  "test-key",
			BaseUrl: "https://api.example.com/v1",
		},
		TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("NewModel returned error: %v", err)
	}
	var mm llms.Model = m
	_ = mm
}

func TestNewModel_LocalConstruction(t *testing.T) {
	// Constructing the model must not dial the local endpoint.
	// Empty host/port fall back to localhost:11434.
	m, err := NewModel(&Config{
		Type:      ModelTypeLocal,
		ModelName: "llama3",
		Local:     &LocalConfig{},
	})
	if err != nil {
		t.Fatalf("NewModel returned error: %v", err)
	}
	if m == nil {
		t.Fatal("NewModel returned nil model")
	}
}

// ---------------------------------------------------------------------------
// Options and applyOptions
// ---------------------------------------------------------------------------

func TestApplyOptions_Empty(t *testing.T) {
	o := applyOptions(nil)
	if o == nil {
		t.Fatal("applyOptions should never return nil")
	}
	if o.httpClient != nil {
		t.Error("default options should have a nil httpClient")
	}
	if len(o.openaiOpts) != 0 {
		t.Error("default options should have no openai options")
	}
	if len(o.ollamaOpts) != 0 {
		t.Error("default options should have no ollama options")
	}
}

func TestApplyOptions_WithHTTPClient(t *testing.T) {
	custom := &http.Client{Timeout: 42 * time.Second}
	o := applyOptions([]Option{WithHTTPClient(custom)})
	if o.httpClient != custom {
		t.Error("WithHTTPClient should store the provided client")
	}
}

func TestApplyOptions_WithOpenaiOptions(t *testing.T) {
	o := applyOptions([]Option{
		WithOpenaiOptions(lcOpenai.WithModel("m1")),
		WithOpenaiOptions(lcOpenai.WithModel("m2")),
	})
	if len(o.openaiOpts) != 2 {
		t.Errorf("len(openaiOpts) = %d, want 2 (options should accumulate)", len(o.openaiOpts))
	}
}

func TestApplyOptions_WithOllamaOptions(t *testing.T) {
	o := applyOptions([]Option{
		WithOllamaOptions(lcOllama.WithModel("m1")),
		WithOllamaOptions(lcOllama.WithModel("m2")),
	})
	if len(o.ollamaOpts) != 2 {
		t.Errorf("len(ollamaOpts) = %d, want 2 (options should accumulate)", len(o.ollamaOpts))
	}
}

// ---------------------------------------------------------------------------
// Cloud model against httptest.Server (base URL injection)
// ---------------------------------------------------------------------------

const openaiChatResponse = `{"id":"chatcmpl-1","object":"chat.completion","created":1700000000,"model":"gpt-test","choices":[{"index":0,"message":{"role":"assistant","content":"hello from openai test"},"finish_reason":"stop"}]}`

// openaiChatRequest is the subset of the request we assert on.
type openaiChatRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func TestNewModel_CloudGenerateContent(t *testing.T) {
	var (
		gotPath string
		gotAuth string
		gotReq  openaiChatRequest
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(openaiChatResponse))
	}))
	defer srv.Close()

	m, err := NewModel(&Config{
		Type:      ModelTypeCloud,
		ModelName: "gpt-test",
		Cloud: &CloudConfig{
			ApiKey:  "secret-key",
			BaseUrl: srv.URL,
		},
	})
	if err != nil {
		t.Fatalf("NewModel returned error: %v", err)
	}

	resp, err := m.GenerateContent(context.Background(), []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, "hi"),
	})
	if err != nil {
		t.Fatalf("GenerateContent returned error: %v", err)
	}

	// Request building checks.
	if gotPath != "/chat/completions" {
		t.Errorf("request path = %q, want %q", gotPath, "/chat/completions")
	}
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

	// Response parsing checks.
	if resp == nil || len(resp.Choices) != 1 {
		t.Fatalf("resp choices = %+v, want exactly 1", resp)
	}
	if got := resp.Choices[0].Content; got != "hello from openai test" {
		t.Errorf("choice content = %q, want %q", got, "hello from openai test")
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

func TestNewModel_CloudCustomHTTPClientUsed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(openaiChatResponse))
	}))
	defer srv.Close()

	transport := &recordingTransport{base: http.DefaultTransport}
	custom := &http.Client{Transport: transport}

	m, err := NewModel(&Config{
		Type:      ModelTypeCloud,
		ModelName: "gpt-test",
		Cloud:     &CloudConfig{ApiKey: "k", BaseUrl: srv.URL},
	}, WithHTTPClient(custom))
	if err != nil {
		t.Fatalf("NewModel returned error: %v", err)
	}

	_, err = m.GenerateContent(context.Background(), []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, "hi"),
	})
	if err != nil {
		t.Fatalf("GenerateContent returned error: %v", err)
	}
	if !transport.used {
		t.Error("the custom HTTP client transport should have been used for the request")
	}
}

// ---------------------------------------------------------------------------
// Local (Ollama) model against httptest.Server (host/port injection)
// ---------------------------------------------------------------------------

func TestNewModel_OllamaGenerateContent(t *testing.T) {
	// Reserve a port on 127.0.0.1 so LocalConfig can reference it explicitly.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen returned error: %v", err)
	}
	port := lis.Addr().(*net.TCPAddr).Port

	var (
		gotPath string
		gotBody struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
	)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(`{"model":"llama3","created_at":"2024-01-01T00:00:00Z","message":{"role":"assistant","content":"local reply"},"done":true}`))
	}))
	srv.Listener = lis
	srv.Start()
	defer srv.Close()

	m, err := NewModel(&Config{
		Type:      ModelTypeLocal,
		ModelName: "llama3",
		Local:     &LocalConfig{Host: "127.0.0.1", Port: int32(port)},
	})
	if err != nil {
		t.Fatalf("NewModel returned error: %v", err)
	}

	resp, err := m.GenerateContent(context.Background(), []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, "hi"),
	})
	if err != nil {
		t.Fatalf("GenerateContent returned error: %v", err)
	}

	if gotPath != "/api/chat" {
		t.Errorf("request path = %q, want %q", gotPath, "/api/chat")
	}
	if gotBody.Model != "llama3" {
		t.Errorf("request model = %q, want %q", gotBody.Model, "llama3")
	}
	if len(gotBody.Messages) != 1 || gotBody.Messages[0].Role != "user" || gotBody.Messages[0].Content != "hi" {
		t.Errorf("request messages = %+v, want one user message 'hi'", gotBody.Messages)
	}
	if resp == nil || len(resp.Choices) != 1 {
		t.Fatalf("resp choices = %+v, want exactly 1", resp)
	}
	if got := resp.Choices[0].Content; got != "local reply" {
		t.Errorf("choice content = %q, want %q", got, "local reply")
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
