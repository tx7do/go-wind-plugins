package langchaingo

import (
	"context"
	"errors"
	"testing"

	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/vectorstores"
)

// mockVectorStore is a deterministic VectorStore for testing.
type mockVectorStore struct {
	gotDocs         []schema.Document
	gotQuery        string
	gotNumDocuments int
	docs            []schema.Document
	ids             []string
	err             error
}

var _ vectorstores.VectorStore = &mockVectorStore{}

func (m *mockVectorStore) AddDocuments(_ context.Context, docs []schema.Document, _ ...vectorstores.Option) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	m.gotDocs = docs
	return m.ids, nil
}

func (m *mockVectorStore) SimilaritySearch(_ context.Context, query string, numDocuments int, _ ...vectorstores.Option) ([]schema.Document, error) {
	if m.err != nil {
		return nil, m.err
	}
	m.gotQuery = query
	m.gotNumDocuments = numDocuments
	return m.docs, nil
}

// ---------------------------------------------------------------------------
// Wrapper functions
// ---------------------------------------------------------------------------

func TestAddDocuments(t *testing.T) {
	store := &mockVectorStore{ids: []string{"id-1", "id-2"}}
	docs := []schema.Document{
		{PageContent: "doc one", Metadata: map[string]any{"source": "a"}},
		{PageContent: "doc two"},
	}

	ids, err := AddDocuments(context.Background(), store, docs)
	if err != nil {
		t.Fatalf("AddDocuments returned error: %v", err)
	}
	if len(ids) != 2 || ids[0] != "id-1" || ids[1] != "id-2" {
		t.Errorf("ids = %v, want [id-1 id-2]", ids)
	}
	if len(store.gotDocs) != 2 || store.gotDocs[0].PageContent != "doc one" {
		t.Errorf("store received docs = %+v, want the two input docs", store.gotDocs)
	}
}

func TestAddDocuments_ErrorPropagation(t *testing.T) {
	store := &mockVectorStore{err: errors.New("store down")}

	if _, err := AddDocuments(context.Background(), store, nil); err == nil {
		t.Error("AddDocuments should propagate store errors")
	}
}

func TestSimilaritySearch(t *testing.T) {
	want := []schema.Document{{PageContent: "matching doc"}}
	store := &mockVectorStore{docs: want}

	got, err := SimilaritySearch(context.Background(), store, "what is go", 3)
	if err != nil {
		t.Fatalf("SimilaritySearch returned error: %v", err)
	}
	if len(got) != 1 || got[0].PageContent != "matching doc" {
		t.Errorf("docs = %+v, want [matching doc]", got)
	}
	if store.gotQuery != "what is go" {
		t.Errorf("store received query = %q, want %q", store.gotQuery, "what is go")
	}
	if store.gotNumDocuments != 3 {
		t.Errorf("store received numDocuments = %d, want 3", store.gotNumDocuments)
	}
}

func TestSimilaritySearch_ErrorPropagation(t *testing.T) {
	store := &mockVectorStore{err: errors.New("search failed")}

	if _, err := SimilaritySearch(context.Background(), store, "q", 1); err == nil {
		t.Error("SimilaritySearch should propagate store errors")
	}
}

// ---------------------------------------------------------------------------
// ToRetriever
// ---------------------------------------------------------------------------

func TestToRetriever(t *testing.T) {
	store := &mockVectorStore{docs: []schema.Document{{PageContent: "retrieved"}}}

	r := ToRetriever(store, 2)
	docs, err := r.GetRelevantDocuments(context.Background(), "query text")
	if err != nil {
		t.Fatalf("GetRelevantDocuments returned error: %v", err)
	}
	if len(docs) != 1 || docs[0].PageContent != "retrieved" {
		t.Errorf("docs = %+v, want [retrieved]", docs)
	}
	if store.gotNumDocuments != 2 {
		t.Errorf("numDocuments passed to the store = %d, want 2", store.gotNumDocuments)
	}
	if store.gotQuery != "query text" {
		t.Errorf("query passed to the store = %q, want %q", store.gotQuery, "query text")
	}
}

// ---------------------------------------------------------------------------
// Option passthroughs
// ---------------------------------------------------------------------------

func TestVectorStoreOptions(t *testing.T) {
	opts := []vectorstores.Option{
		WithNameSpace("ns"),
		WithScoreThreshold(0.5),
		WithFilters(map[string]any{"k": "v"}),
		WithEmbedder(nil),
		WithDeduplicater(func(ctx context.Context, doc schema.Document) bool { return false }),
	}
	for i, opt := range opts {
		if opt == nil {
			t.Errorf("option %d should not be nil", i)
		}
	}
}
