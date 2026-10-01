package langchaingo

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/embeddings"
)

// mockEmbedderClient is a deterministic EmbedderClient for testing.
type mockEmbedderClient struct {
	gotCalls [][]string
	err      error
}

var _ embeddings.EmbedderClient = &mockEmbedderClient{}

func (m *mockEmbedderClient) CreateEmbedding(_ context.Context, texts []string) ([][]float32, error) {
	if m.err != nil {
		return nil, m.err
	}
	// Copy to detach from the caller's slice.
	copied := make([]string, len(texts))
	copy(copied, texts)
	m.gotCalls = append(m.gotCalls, copied)
	return vectorsFor(texts), nil
}

// vectorsFor returns a distinct single-element vector per input text so order
// and completeness can be asserted.
func vectorsFor(texts []string) [][]float32 {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{float32(i + 1)}
	}
	return out
}

// ---------------------------------------------------------------------------
// NewEmbedder
// ---------------------------------------------------------------------------

func TestNewEmbedder(t *testing.T) {
	e, err := NewEmbedder(&mockEmbedderClient{})
	if err != nil {
		t.Fatalf("NewEmbedder returned error: %v", err)
	}
	if e == nil {
		t.Fatal("NewEmbedder returned nil embedder")
	}
}

func TestEmbedQuery(t *testing.T) {
	client := mockEmbedderClient{}

	vec, err := EmbedQuery(context.Background(), &client, "hello world")
	if err != nil {
		t.Fatalf("EmbedQuery returned error: %v", err)
	}
	if len(vec) != 1 || vec[0] != 1 {
		t.Errorf("vector = %v, want [1]", vec)
	}
	if len(client.gotCalls) != 1 {
		t.Fatalf("CreateEmbedding calls = %d, want 1", len(client.gotCalls))
	}
	if got := client.gotCalls[0]; len(got) != 1 || got[0] != "hello world" {
		t.Errorf("embedded texts = %v, want [hello world]", got)
	}
}

func TestEmbedQuery_StripsNewLinesByDefault(t *testing.T) {
	client := mockEmbedderClient{}

	if _, err := EmbedQuery(context.Background(), &client, "hello\nworld"); err != nil {
		t.Fatalf("EmbedQuery returned error: %v", err)
	}
	if got := client.gotCalls[0][0]; got != "hello world" {
		t.Errorf("text = %q, want newlines stripped: %q", got, "hello world")
	}
}

func TestNewEmbedder_WithStripNewLinesFalse(t *testing.T) {
	client := mockEmbedderClient{}

	e, err := NewEmbedder(&client, WithStripNewLines(false))
	if err != nil {
		t.Fatalf("NewEmbedder returned error: %v", err)
	}

	if _, err := e.EmbedQuery(context.Background(), "hello\nworld"); err != nil {
		t.Fatalf("EmbedQuery returned error: %v", err)
	}
	if got := client.gotCalls[0][0]; got != "hello\nworld" {
		t.Errorf("text = %q, want newlines preserved: %q", got, "hello\nworld")
	}
}

func TestEmbedQuery_ErrorPropagation(t *testing.T) {
	client := mockEmbedderClient{err: errors.New("embedding backend down")}

	if _, err := EmbedQuery(context.Background(), &client, "text"); err == nil {
		t.Error("EmbedQuery should propagate client errors")
	}
}

// ---------------------------------------------------------------------------
// EmbedDocuments batching
// ---------------------------------------------------------------------------

func TestEmbedDocuments(t *testing.T) {
	client := mockEmbedderClient{}

	vecs, err := EmbedDocuments(context.Background(), &client, []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("EmbedDocuments returned error: %v", err)
	}
	if len(vecs) != 3 {
		t.Fatalf("len(vectors) = %d, want 3", len(vecs))
	}
	// A single call with the default batch size (512).
	if len(client.gotCalls) != 1 {
		t.Fatalf("CreateEmbedding calls = %d, want 1", len(client.gotCalls))
	}
}

func TestNewEmbedder_WithBatchSize(t *testing.T) {
	client := mockEmbedderClient{}

	e, err := NewEmbedder(&client, WithBatchSize(2))
	if err != nil {
		t.Fatalf("NewEmbedder returned error: %v", err)
	}

	vecs, err := e.EmbedDocuments(context.Background(), []string{"a", "b", "c", "d", "e"})
	if err != nil {
		t.Fatalf("EmbedDocuments returned error: %v", err)
	}
	if len(vecs) != 5 {
		t.Fatalf("len(vectors) = %d, want 5", len(vecs))
	}

	// 5 documents with batch size 2 must produce calls of 2, 2 and 1.
	if len(client.gotCalls) != 3 {
		t.Fatalf("CreateEmbedding calls = %d, want 3", len(client.gotCalls))
	}
	wantSizes := []int{2, 2, 1}
	for i, want := range wantSizes {
		if got := len(client.gotCalls[i]); got != want {
			t.Errorf("batch %d size = %d, want %d", i, got, want)
		}
	}
}

func TestEmbedDocuments_StripsNewLines(t *testing.T) {
	client := mockEmbedderClient{}

	if _, err := EmbedDocuments(context.Background(), &client, []string{"a\nb"}); err != nil {
		t.Fatalf("EmbedDocuments returned error: %v", err)
	}
	if got := client.gotCalls[0][0]; strings.Contains(got, "\n") {
		t.Errorf("text = %q, want newlines stripped", got)
	}
}
