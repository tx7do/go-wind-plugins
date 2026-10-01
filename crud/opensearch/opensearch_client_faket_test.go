package opensearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	opensearchV4 "github.com/opensearch-project/opensearch-go/v4"
	paginationV1 "github.com/tx7do/go-wind-plugins/crud/api/gen/go/pagination/v1"
)

// paginationV1PagingRequestFixture builds a page/size based paging request.
func paginationV1PagingRequestFixture(page, pageSize uint32) *paginationV1.PagingRequest {
	return &paginationV1.PagingRequest{Page: &page, PageSize: &pageSize}
}

// pagingReqWithOrderByFixture builds a paging request with an order-by string.
func pagingReqWithOrderByFixture(orderBy string) *paginationV1.PagingRequest {
	return &paginationV1.PagingRequest{OrderBy: &orderBy}
}

// ---------------------------------------------------------------------------
// Fake transport: every request is answered from a scripted queue, so the
// whole client surface runs without a cluster.
// ---------------------------------------------------------------------------

type fakeResponse struct {
	status int
	body   string
}

type fakeTransport struct {
	mu        sync.Mutex
	responses []fakeResponse
	requests  []*http.Request
	err       error // when set, RoundTrip fails instead of answering
}

func (f *fakeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r)
	if f.err != nil {
		return nil, f.err
	}
	if len(f.responses) == 0 {
		return nil, fmt.Errorf("fake transport: no scripted response for %s %s", r.Method, r.URL.Path)
	}
	fr := f.responses[0]
	f.responses = f.responses[1:]
	return &http.Response{
		StatusCode: fr.status,
		Body:       io.NopCloser(strings.NewReader(fr.body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Request:    r,
	}, nil
}

func (f *fakeTransport) enqueue(status int, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.responses = append(f.responses, fakeResponse{status: status, body: body})
}

func (f *fakeTransport) lastRequest() *http.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		return nil
	}
	return f.requests[len(f.requests)-1]
}

// ok is the canonical success body most endpoints return.
const okBody = `{"acknowledged":true}`

func newFakeOSClient(t *testing.T) (*Client, *fakeTransport) {
	t.Helper()
	ft := &fakeTransport{}
	c, err := NewOpenSearchClient(
		WithAddresses("http://fake.local:9200"),
		WithTransport(ft),
		WithDisableRetry(true),
		WithMaxRetries(1),
	)
	if err != nil {
		t.Fatalf("NewOpenSearchClient error: %v", err)
	}
	return c, ft
}

// TestOptions_All applies every Option and asserts the config mutation.
func TestOptions_All(t *testing.T) {
	c := &Client{options: &opensearchV4.Config{}}

	WithAddresses("http://a:9200")(c)
	if len(c.options.Addresses) != 1 || c.options.Addresses[0] != "http://a:9200" {
		t.Error("WithAddresses did not set addresses")
	}
	WithUsername("u")(c)
	if c.options.Username != "u" {
		t.Error("WithUsername did not set username")
	}
	WithPassword("p")(c)
	if c.options.Password != "p" {
		t.Error("WithPassword did not set password")
	}
	WithEnableMetrics(true)(c)
	if !c.options.EnableMetrics {
		t.Error("WithEnableMetrics did not set flag")
	}
	WithEnableDebugLogger(true)(c)
	if !c.options.EnableDebugLogger {
		t.Error("WithEnableDebugLogger did not set flag")
	}
	WithDiscoverNodesOnStart(true)(c)
	if c.options.DiscoverNodesOnStart == nil || !*c.options.DiscoverNodesOnStart {
		t.Error("WithDiscoverNodesOnStart did not set flag")
	}
	WithDiscoverNodesInterval(1e9)(c)
	if c.options.DiscoverNodesInterval != 1e9 {
		t.Error("WithDiscoverNodesInterval did not set interval")
	}
	WithDisableRetry(true)(c)
	if !c.options.DisableRetry {
		t.Error("WithDisableRetry did not set flag")
	}
	WithEnableRetryOnTimeout(true)(c)
	if !c.options.EnableRetryOnTimeout {
		t.Error("WithEnableRetryOnTimeout did not set flag")
	}
	WithMaxRetries(3)(c)
	if c.options.MaxRetries != 3 {
		t.Error("WithMaxRetries did not set value")
	}
	WithCompressRequestBody(true)(c)
	if !c.options.CompressRequestBody {
		t.Error("WithCompressRequestBody did not set flag")
	}
	tr := &fakeTransport{}
	WithTransport(tr)(c)
	if c.options.Transport == nil {
		t.Error("WithTransport did not set transport")
	}
	WithRetryOnStatus(500, 502)(c)
	if len(c.options.RetryOnStatus) != 2 {
		t.Error("WithRetryOnStatus did not set statuses")
	}
	WithRetryBackoff(func(int) time.Duration { return 0 })(c)
	if c.options.RetryBackoff == nil {
		t.Error("WithRetryBackoff did not set backoff")
	}
	WithHeader(http.Header{"X-Test": {"1"}})(c)
	if c.options.Header == nil {
		t.Error("WithHeader did not set header")
	}
	WithCACert([]byte("cert"))(c)
	if string(c.options.CACert) != "cert" {
		t.Error("WithCACert did not set cert")
	}
	WithLogger(nil)(c)
}

// ---------------------------------------------------------------------------
// Index lifecycle
// ---------------------------------------------------------------------------

func TestIndexExists(t *testing.T) {
	c, ft := newFakeOSClient(t)

	ft.enqueue(200, okBody)
	exists, err := c.IndexExists(context.Background(), "idx")
	if err != nil || !exists {
		t.Fatalf("IndexExists(200) = %v, %v", exists, err)
	}
	if got := ft.lastRequest(); got == nil || got.Method != http.MethodHead || !strings.HasSuffix(got.URL.Path, "/idx") {
		t.Fatalf("unexpected request: %v", got)
	}

	ft.enqueue(404, "")
	exists, err = c.IndexExists(context.Background(), "idx")
	if err != nil || exists {
		t.Fatalf("IndexExists(404) = %v, %v", exists, err)
	}
}

func TestCreateIndex_Fake(t *testing.T) {
	ctx := context.Background()

	// success path: HEAD 404 then PUT 200
	c, ft := newFakeOSClient(t)
	ft.enqueue(404, "")
	ft.enqueue(200, okBody)
	if err := c.CreateIndex(ctx, "idx", `{"mappings":{"properties":{"a":{"type":"text"}}}}`, `{"settings":{"number_of_shards":1}}`); err != nil {
		t.Fatalf("CreateIndex error: %v", err)
	}

	// already exists
	ft.enqueue(200, okBody)
	if err := c.CreateIndex(ctx, "idx", "", ""); !errors.Is(err, ErrIndexAlreadyExists) {
		t.Fatalf("CreateIndex(exists) = %v", err)
	}

	// malformed mapping surfaces the merge error
	ft.enqueue(404, "")
	if err := c.CreateIndex(ctx, "idx", "{bad json", ""); err == nil {
		t.Fatal("malformed mapping must fail")
	}

	// creation answered with an error document
	ft.enqueue(404, "")
	ft.enqueue(400, `{"error":{"reason":"bad mapping"},"status":400}`)
	if err := c.CreateIndex(ctx, "idx", "", ""); !errors.Is(err, ErrCreateIndex) {
		t.Fatalf("CreateIndex(error resp) = %v", err)
	}

	// unparseable error body
	ft.enqueue(404, "")
	ft.enqueue(400, "not-json")
	if err := c.CreateIndex(ctx, "idx", "", ""); !errors.Is(err, ErrUnmarshalResponse) {
		t.Fatalf("CreateIndex(bad error body) = %v", err)
	}
}

func TestDeleteIndex_Fake(t *testing.T) {
	ctx := context.Background()
	c, ft := newFakeOSClient(t)

	ft.enqueue(404, "")
	if err := c.DeleteIndex(ctx, "idx"); !errors.Is(err, ErrIndexNotFound) {
		t.Fatalf("DeleteIndex(missing) = %v", err)
	}

	ft.enqueue(200, okBody)
	ft.enqueue(200, okBody)
	if err := c.DeleteIndex(ctx, "idx"); err != nil {
		t.Fatalf("DeleteIndex error: %v", err)
	}

	ft.enqueue(200, okBody)
	ft.enqueue(500, `{"error":{"reason":"locked"},"status":500}`)
	if err := c.DeleteIndex(ctx, "idx"); !errors.Is(err, ErrDeleteIndex) {
		t.Fatalf("DeleteIndex(error resp) = %v", err)
	}
}

// ---------------------------------------------------------------------------
// Documents
// ---------------------------------------------------------------------------

func TestInsertDocument_Fake(t *testing.T) {
	ctx := context.Background()
	c, ft := newFakeOSClient(t)

	ft.enqueue(200, okBody)
	if err := c.InsertDocument(ctx, "idx", "1", map[string]any{"a": 1}); err != nil {
		t.Fatalf("InsertDocument error: %v", err)
	}

	ft.enqueue(409, `{"error":"conflict"}`)
	if err := c.InsertDocument(ctx, "idx", "1", map[string]any{"a": 1}); !errors.Is(err, ErrDocumentConflict) {
		t.Fatalf("InsertDocument(409) = %v", err)
	}

	ft.enqueue(400, `{"error":"bad"}`)
	if err := c.InsertDocument(ctx, "idx", "1", map[string]any{"a": 1}); !errors.Is(err, ErrInsertDocument) {
		t.Fatalf("InsertDocument(400) = %v", err)
	}

	// unmarshalable payload fails before the request
	if err := c.InsertDocument(ctx, "idx", "1", map[string]any{"ch": make(chan int)}); err == nil {
		t.Fatal("unmarshalable payload must fail")
	}
}

func TestBatchInsertDocument_Fake(t *testing.T) {
	ctx := context.Background()
	c, ft := newFakeOSClient(t)

	ft.enqueue(200, `{"errors":false,"items":[]}`)
	if err := c.BatchInsertDocument(ctx, "idx", []any{map[string]any{"a": 1}, map[string]any{"b": 2}}, []string{"1", "2"}); err != nil {
		t.Fatalf("BatchInsertDocument error: %v", err)
	}

	ft.enqueue(500, `{"error":"bulk boom"}`)
	if err := c.BatchInsertDocument(ctx, "idx", []any{map[string]any{"a": 1}}, nil); !errors.Is(err, ErrBatchInsertDocument) {
		t.Fatalf("BatchInsertDocument(error resp) = %v", err)
	}
}

func TestMultiGet(t *testing.T) {
	ctx := context.Background()
	c, ft := newFakeOSClient(t)

	// empty ids short-circuits
	got, err := c.MultiGet(ctx, "idx", nil, nil)
	if err != nil || got != nil {
		t.Fatalf("MultiGet(empty) = %v, %v", got, err)
	}

	// ids that are all empty short-circuit as well
	got, err = c.MultiGet(ctx, "idx", []string{"", ""}, nil)
	if err != nil || got != nil {
		t.Fatalf("MultiGet(blank ids) = %v, %v", got, err)
	}

	ft.enqueue(200, `{"docs":[{"found":true,"_source":{"a":1}},{"found":false}]}`)
	got, err = c.MultiGet(ctx, "idx", []string{"1", "2", "3"}, []string{"a"})
	if err != nil {
		t.Fatalf("MultiGet error: %v", err)
	}
	if len(got) != 3 || string(got[0]) != `{"a":1}` || got[1] != nil || got[2] != nil {
		t.Fatalf("MultiGet results = %v", got)
	}

	ft.enqueue(400, `{"error":{"reason":"bad"},"status":400}`)
	if _, err := c.MultiGet(ctx, "idx", []string{"1"}, nil); !errors.Is(err, ErrGetDocument) {
		t.Fatalf("MultiGet(error resp) = %v", err)
	}
}

func TestBatchUpdateDocument(t *testing.T) {
	ctx := context.Background()
	c, ft := newFakeOSClient(t)

	// ids/data length mismatch
	if err := c.BatchUpdateDocument(ctx, "idx", []any{map[string]any{"a": 1}}, []string{"1", "2"}); err == nil {
		t.Fatal("length mismatch must fail")
	}

	// all items missing ids
	if err := c.BatchUpdateDocument(ctx, "idx", []any{map[string]any{"a": 1}}, nil); err == nil {
		t.Fatal("items without ids must fail")
	}

	ft.enqueue(200, okBody)
	if err := c.BatchUpdateDocument(ctx, "idx", []any{map[string]any{"a": 1}}, []string{"1"}); err != nil {
		t.Fatalf("BatchUpdateDocument error: %v", err)
	}

	ft.enqueue(500, `{"error":"bulk boom"}`)
	if err := c.BatchUpdateDocument(ctx, "idx", []any{map[string]any{"a": 1}}, []string{"1"}); !errors.Is(err, ErrBatchInsertDocument) {
		t.Fatalf("BatchUpdateDocument(error resp) = %v", err)
	}
}

func TestBatchDeleteDocument(t *testing.T) {
	ctx := context.Background()
	c, ft := newFakeOSClient(t)

	// only blank ids
	if err := c.BatchDeleteDocument(ctx, "idx", []string{"", " "}); err == nil {
		t.Fatal("blank-only ids must fail")
	}

	ft.enqueue(200, okBody)
	if err := c.BatchDeleteDocument(ctx, "idx", []string{"1", ""}); err != nil {
		t.Fatalf("BatchDeleteDocument error: %v", err)
	}

	ft.enqueue(500, `{"error":"bulk boom"}`)
	if err := c.BatchDeleteDocument(ctx, "idx", []string{"1"}); !errors.Is(err, ErrBatchInsertDocument) {
		t.Fatalf("BatchDeleteDocument(error resp) = %v", err)
	}
}

func TestUpdateAndDeleteByQuery(t *testing.T) {
	ctx := context.Background()
	c, ft := newFakeOSClient(t)

	ft.enqueue(200, okBody)
	if err := c.UpdateByQuery(ctx, "idx", `{"query":{"match_all":{}}}`); err != nil {
		t.Fatalf("UpdateByQuery error: %v", err)
	}
	ft.enqueue(500, `{"error":{"reason":"boom"},"status":500}`)
	if err := c.UpdateByQuery(ctx, "idx", "{}"); !errors.Is(err, ErrUpdateDocument) {
		t.Fatalf("UpdateByQuery(error resp) = %v", err)
	}
	ft.enqueue(500, "not-json")
	if err := c.UpdateByQuery(ctx, "idx", "{}"); !errors.Is(err, ErrUnmarshalResponse) {
		t.Fatalf("UpdateByQuery(bad error body) = %v", err)
	}

	ft.enqueue(200, okBody)
	if err := c.DeleteByQuery(ctx, "idx", `{"query":{"match_all":{}}}`); err != nil {
		t.Fatalf("DeleteByQuery error: %v", err)
	}
	ft.enqueue(500, `{"error":{"reason":"boom"},"status":500}`)
	if err := c.DeleteByQuery(ctx, "idx", "{}"); !errors.Is(err, ErrDeleteDocument) {
		t.Fatalf("DeleteByQuery(error resp) = %v", err)
	}
}

func TestUpdateDocument(t *testing.T) {
	ctx := context.Background()
	c, ft := newFakeOSClient(t)

	ft.enqueue(200, okBody)
	if err := c.UpdateDocument(ctx, "idx", "1", map[string]any{"a": 2}); err != nil {
		t.Fatalf("UpdateDocument error: %v", err)
	}

	ft.enqueue(404, `{"error":{"reason":"missing"}}`)
	if err := c.UpdateDocument(ctx, "idx", "1", map[string]any{"a": 2}); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("UpdateDocument(404) = %v", err)
	}

	ft.enqueue(400, `{"error":{"reason":"bad"}}`)
	if err := c.UpdateDocument(ctx, "idx", "1", map[string]any{"a": 2}); !errors.Is(err, ErrUpdateDocument) {
		t.Fatalf("UpdateDocument(400) = %v", err)
	}
}

func TestGetDocument_Fake(t *testing.T) {
	ctx := context.Background()
	c, ft := newFakeOSClient(t)

	var out struct {
		A int `json:"a"`
	}
	ft.enqueue(200, `{"_index":"idx","_id":"1","found":true,"_source":{"a":7}}`)
	if err := c.GetDocument(ctx, "idx", "1", []string{"a"}, &out); err != nil {
		t.Fatalf("GetDocument error: %v", err)
	}
	if out.A != 7 {
		t.Fatalf("GetDocument decoded %+v", out)
	}

	ft.enqueue(404, `{"error":{"reason":"missing"},"status":404}`)
	if err := c.GetDocument(ctx, "idx", "1", nil, &out); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("GetDocument(404) = %v", err)
	}

	ft.enqueue(500, `{"error":{"reason":"boom"},"status":500}`)
	if err := c.GetDocument(ctx, "idx", "1", nil, &out); !errors.Is(err, ErrGetDocument) {
		t.Fatalf("GetDocument(500) = %v", err)
	}

	ft.enqueue(200, `{"_source":"not-an-object"}`)
	if err := c.GetDocument(ctx, "idx", "1", nil, &out); err == nil {
		t.Fatal("undecodable source must fail")
	}
}

// ---------------------------------------------------------------------------
// Templates and ISM policies
// ---------------------------------------------------------------------------

func TestTemplates(t *testing.T) {
	ctx := context.Background()
	c, ft := newFakeOSClient(t)

	ft.enqueue(200, okBody)
	if err := c.CreateIndexTemplate(ctx, "tpl", `{"index_patterns":["i*"]}`); err != nil {
		t.Fatalf("CreateIndexTemplate error: %v", err)
	}
	ft.enqueue(400, `{"error":"bad"}`)
	if err := c.CreateIndexTemplate(ctx, "tpl", "{}"); !errors.Is(err, ErrCreateTemplate) {
		t.Fatalf("CreateIndexTemplate(error resp) = %v", err)
	}

	ft.enqueue(200, okBody)
	if exists, err := c.ExistsIndexTemplate(ctx, "tpl"); err != nil || !exists {
		t.Fatalf("ExistsIndexTemplate = %v, %v", exists, err)
	}
	ft.enqueue(404, "")
	if exists, err := c.ExistsIndexTemplate(ctx, "tpl"); err != nil || exists {
		t.Fatalf("ExistsIndexTemplate(404) = %v, %v", exists, err)
	}

	ft.enqueue(200, okBody)
	if err := c.DeleteIndexTemplate(ctx, "tpl"); err != nil {
		t.Fatalf("DeleteIndexTemplate error: %v", err)
	}
	ft.enqueue(400, `{"error":"bad"}`)
	if err := c.DeleteIndexTemplate(ctx, "tpl"); !errors.Is(err, ErrDeleteTemplate) {
		t.Fatalf("DeleteIndexTemplate(error resp) = %v", err)
	}

	ft.enqueue(200, okBody)
	if err := c.CreateComponentTemplate(ctx, "ctpl", "{}"); err != nil {
		t.Fatalf("CreateComponentTemplate error: %v", err)
	}
	ft.enqueue(400, `{"error":"bad"}`)
	if err := c.CreateComponentTemplate(ctx, "ctpl", "{}"); !errors.Is(err, ErrCreateTemplate) {
		t.Fatalf("CreateComponentTemplate(error resp) = %v", err)
	}

	ft.enqueue(200, okBody)
	if err := c.DeleteComponentTemplate(ctx, "ctpl"); err != nil {
		t.Fatalf("DeleteComponentTemplate error: %v", err)
	}
	ft.enqueue(400, `{"error":"bad"}`)
	if err := c.DeleteComponentTemplate(ctx, "ctpl"); !errors.Is(err, ErrDeleteTemplate) {
		t.Fatalf("DeleteComponentTemplate(error resp) = %v", err)
	}

	ft.enqueue(200, okBody)
	if exists, err := c.ExistsComponentTemplate(ctx, "ctpl"); err != nil || !exists {
		t.Fatalf("ExistsComponentTemplate = %v, %v", exists, err)
	}
}

func TestISMPolicies(t *testing.T) {
	ctx := context.Background()
	c, ft := newFakeOSClient(t)

	ft.enqueue(200, `{"policy":{"policy_id":"p"}}`)
	if err := c.CreateISMPolicy(ctx, "p", `{"policy":{}}`); err != nil {
		t.Fatalf("CreateISMPolicy error: %v", err)
	}
	ft.enqueue(400, `{"error":{"reason":"bad"}}`)
	if err := c.CreateISMPolicy(ctx, "p", "{}"); !errors.Is(err, ErrCreateISMPolicy) {
		t.Fatalf("CreateISMPolicy(error resp) = %v", err)
	}
	ft.enqueue(400, "not-json")
	if err := c.CreateISMPolicy(ctx, "p", "{}"); !errors.Is(err, ErrUnmarshalResponse) {
		t.Fatalf("CreateISMPolicy(bad error body) = %v", err)
	}

	ft.enqueue(200, `{"deleted":true}`)
	if err := c.DeleteISMPolicy(ctx, "p"); err != nil {
		t.Fatalf("DeleteISMPolicy error: %v", err)
	}
	ft.enqueue(404, `{"error":{"reason":"missing"}}`)
	if err := c.DeleteISMPolicy(ctx, "p"); !errors.Is(err, ErrDeleteISMPolicy) {
		t.Fatalf("DeleteISMPolicy(error resp) = %v", err)
	}

	ft.enqueue(200, `{"policy":{"default_state":"x"}}`)
	policy, err := c.GetISMPolicy(ctx, "p")
	if err != nil || policy == nil {
		t.Fatalf("GetISMPolicy = %v, %v", policy, err)
	}
	ft.enqueue(404, `{"error":{"reason":"missing"}}`)
	if _, err := c.GetISMPolicy(ctx, "p"); !errors.Is(err, ErrRequestFailed) {
		t.Fatalf("GetISMPolicy(error resp) = %v", err)
	}
}

// ---------------------------------------------------------------------------
// Search surface
// ---------------------------------------------------------------------------

const searchHits = `{"took":1,"hits":{"total":{"value":1},"hits":[{"_index":"idx","_id":"1","_source":{"a":1}}]}}`

func TestSearch(t *testing.T) {
	ctx := context.Background()
	c, ft := newFakeOSClient(t)

	page := uint32(1)
	pageSize := uint32(10)
	ft.enqueue(200, searchHits)
	res, err := c.Search(ctx, "idx", paginationV1PagingRequestFixture(page, pageSize))
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if res == nil || len(res.Hits.Hits) != 1 {
		t.Fatalf("Search returned %+v", res)
	}

	ft.enqueue(500, `{"error":"boom"}`)
	if _, err := c.Search(ctx, "idx", paginationV1PagingRequestFixture(page, pageSize)); !errors.Is(err, ErrSearchDocument) {
		t.Fatalf("Search(error resp) = %v", err)
	}

	// malformed order-by fails before any request
	if _, err := c.Search(ctx, "idx", pagingReqWithOrderByFixture("[")); err == nil {
		t.Fatal("malformed order-by must fail")
	}
}

func TestSearchWithHighlightAndBody(t *testing.T) {
	ctx := context.Background()
	c, ft := newFakeOSClient(t)

	ft.enqueue(200, searchHits)
	res, err := c.SearchWithHighlight(ctx, "idx",
		map[string]any{"match_all": struct{}{}},
		map[string]any{"fields": map[string]any{"a": map[string]any{}}},
		[]string{"a"}, map[string]bool{"a": true}, 0, 10)
	if err != nil || res == nil {
		t.Fatalf("SearchWithHighlight = %v, %v", res, err)
	}

	ft.enqueue(500, `{"error":"boom"}`)
	if _, err := c.SearchWithHighlight(ctx, "idx", nil, nil, nil, nil, 0, 10); !errors.Is(err, ErrSearchDocument) {
		t.Fatalf("SearchWithHighlight(error resp) = %v", err)
	}

	ft.enqueue(200, searchHits)
	res, err = c.SearchWithBody(ctx, "idx", map[string]any{"query": map[string]any{"match_all": struct{}{}}})
	if err != nil || res == nil {
		t.Fatalf("SearchWithBody = %v, %v", res, err)
	}
	ft.enqueue(500, `{"error":"boom"}`)
	if _, err := c.SearchWithBody(ctx, "idx", nil); !errors.Is(err, ErrSearchDocument) {
		t.Fatalf("SearchWithBody(error resp) = %v", err)
	}
}

func TestCount(t *testing.T) {
	ctx := context.Background()
	c, ft := newFakeOSClient(t)

	if _, err := c.Count(ctx, "", nil); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("Count(empty index) = %v", err)
	}

	ft.enqueue(200, `{"count":42}`)
	n, err := c.Count(ctx, "idx", map[string]any{"query": map[string]any{"match_all": struct{}{}}})
	if err != nil || n != 42 {
		t.Fatalf("Count = %d, %v", n, err)
	}

	ft.enqueue(400, `{"error":{"reason":"bad"},"status":400}`)
	if _, err := c.Count(ctx, "idx", nil); !errors.Is(err, ErrSearchDocument) {
		t.Fatalf("Count(error resp) = %v", err)
	}
}

func TestScroll(t *testing.T) {
	ctx := context.Background()
	c, ft := newFakeOSClient(t)

	ft.enqueue(200, searchHits)
	res, err := c.SearchScroll(ctx, "scroll-1", "1m")
	if err != nil || res == nil {
		t.Fatalf("SearchScroll = %v, %v", res, err)
	}
	ft.enqueue(400, `{"error":{"reason":"bad"},"status":400}`)
	if _, err := c.SearchScroll(ctx, "scroll-1", "1m"); !errors.Is(err, ErrSearchDocument) {
		t.Fatalf("SearchScroll(error resp) = %v", err)
	}

	ft.enqueue(200, `{"succeeded":true}`)
	if err := c.ClearScroll(ctx, "scroll-1"); err != nil {
		t.Fatalf("ClearScroll error: %v", err)
	}
	ft.enqueue(400, `{"error":{"reason":"bad"},"status":400}`)
	if err := c.ClearScroll(ctx, "scroll-1"); !errors.Is(err, ErrSearchDocument) {
		t.Fatalf("ClearScroll(error resp) = %v", err)
	}
}

// ---------------------------------------------------------------------------
// SQL surface
// ---------------------------------------------------------------------------

func TestSearchBySQL(t *testing.T) {
	ctx := context.Background()
	c, ft := newFakeOSClient(t)

	ft.enqueue(200, `{"schema":[{"name":"a","type":"long"}],"total":1,"datarows":[[7]],"status":200}`)
	res, err := c.SearchBySQL(ctx, "SELECT a FROM idx")
	if err != nil {
		t.Fatalf("SearchBySQL error: %v", err)
	}
	if res.Total != 1 || len(res.Datarows) != 1 || res.Datarows[0][0] != float64(7) {
		t.Fatalf("SearchBySQL result = %+v", res)
	}

	ft.enqueue(400, `{"error":"bad sql"}`)
	if _, err := c.SearchBySQL(ctx, "BAD"); !errors.Is(err, ErrSearchDocument) {
		t.Fatalf("SearchBySQL(error resp) = %v", err)
	}

	ft.enqueue(200, "not-json")
	if _, err := c.SearchBySQL(ctx, "SELECT 1"); err == nil {
		t.Fatal("undecodable sql result must fail")
	}

	// SearchBySQLTo maps datarows through the schema onto a struct slice
	ft.enqueue(200, `{"schema":[{"name":"a","type":"long"}],"datarows":[[7]],"status":200}`)
	var rows []struct {
		A int `json:"a"`
	}
	if err := c.SearchBySQLTo(ctx, "SELECT a FROM idx", &rows); err != nil {
		t.Fatalf("SearchBySQLTo error: %v", err)
	}
	if len(rows) != 1 || rows[0].A != 7 {
		t.Fatalf("SearchBySQLTo rows = %+v", rows)
	}
}

func TestSQLToDSLAndHighlight(t *testing.T) {
	ctx := context.Background()
	c, ft := newFakeOSClient(t)

	ft.enqueue(200, `{"query":{"match_all":{}}}`)
	dsl, err := c.SQLToDSL(ctx, "SELECT * FROM idx")
	if err != nil || dsl == nil {
		t.Fatalf("SQLToDSL = %v, %v", dsl, err)
	}

	// highlight flow: translate first, then the real search
	ft.enqueue(200, `{"query":{"match_all":{}}}`)
	ft.enqueue(200, searchHits)
	res, err := c.SearchBySQLWithHighlight(ctx, "idx", "SELECT * FROM idx", []string{"a"})
	if err != nil || res == nil {
		t.Fatalf("SearchBySQLWithHighlight = %v, %v", res, err)
	}

	// search leg fails when the search endpoint answers an error
	ft.enqueue(200, `{"query":{}}`)
	ft.enqueue(500, "boom")
	if _, err := c.SearchBySQLWithHighlight(ctx, "idx", "SELECT * FROM idx", nil); err == nil {
		t.Fatal("expected search leg failure")
	}
}

func TestQueryWithSQLPagination_Fake(t *testing.T) {
	ctx := context.Background()
	c, ft := newFakeOSClient(t)

	page := uint32(1)
	pageSize := uint32(5)
	ft.enqueue(200, `{"schema":[{"name":"a","type":"long"}],"datarows":[[7]],"status":200}`)
	res, err := c.QueryWithSQLPagination(ctx, "idx", paginationV1PagingRequestFixture(page, pageSize))
	if err != nil || res == nil {
		t.Fatalf("QueryWithSQLPagination = %v, %v", res, err)
	}

	var rows []struct {
		A int `json:"a"`
	}
	ft.enqueue(200, `{"schema":[{"name":"a","type":"long"}],"datarows":[[8]],"status":200}`)
	if err := c.QueryWithSQLPaginationTo(ctx, "idx", paginationV1PagingRequestFixture(page, pageSize), &rows); err != nil {
		t.Fatalf("QueryWithSQLPaginationTo error: %v", err)
	}
	if len(rows) != 1 || rows[0].A != 8 {
		t.Fatalf("QueryWithSQLPaginationTo rows = %+v", rows)
	}
}

// ---------------------------------------------------------------------------
// Index administration
// ---------------------------------------------------------------------------

func TestIndexAdmin(t *testing.T) {
	ctx := context.Background()
	c, ft := newFakeOSClient(t)

	// refresh/open/close/flush success and failure
	for _, call := range []func() error{
		func() error { return c.RefreshIndex(ctx, "idx") },
		func() error { return c.OpenIndex(ctx, "idx") },
		func() error { return c.CloseIndex(ctx, "idx") },
		func() error { return c.FlushIndex(ctx, "idx") },
	} {
		ft.enqueue(200, okBody)
		if err := call(); err != nil {
			t.Fatalf("admin op error: %v", err)
		}
		ft.enqueue(500, "boom")
		if err := call(); err == nil {
			t.Fatal("admin op with 500 must fail")
		}
	}

	// aliases
	ft.enqueue(200, okBody)
	if err := c.CreateAlias(ctx, "al", "idx", ""); err != nil {
		t.Fatalf("CreateAlias error: %v", err)
	}
	ft.enqueue(200, okBody)
	if err := c.DeleteAlias(ctx, "al", "idx"); err != nil {
		t.Fatalf("DeleteAlias error: %v", err)
	}
	ft.enqueue(200, `{"idx":{"aliases":{"al":{}}}}`)
	aliases, err := c.GetAlias(ctx, "al")
	if err != nil || aliases == nil {
		t.Fatalf("GetAlias = %v, %v", aliases, err)
	}
	ft.enqueue(200, okBody)
	if exists, err := c.ExistsAlias(ctx, "al"); err != nil || !exists {
		t.Fatalf("ExistsAlias = %v, %v", exists, err)
	}

	// mappings and settings
	ft.enqueue(200, `{"idx":{"mappings":{"properties":{}}}}`)
	m, err := c.GetMapping(ctx, "idx")
	if err != nil || m == nil {
		t.Fatalf("GetMapping = %v, %v", m, err)
	}
	ft.enqueue(200, okBody)
	if err := c.PutMapping(ctx, "idx", `{"properties":{"a":{"type":"text"}}}`); err != nil {
		t.Fatalf("PutMapping error: %v", err)
	}
	ft.enqueue(200, `{"idx":{"settings":{"index":{}}}}`)
	s, err := c.GetSettings(ctx, "idx")
	if err != nil || s == nil {
		t.Fatalf("GetSettings = %v, %v", s, err)
	}
	ft.enqueue(200, okBody)
	if err := c.PutSettings(ctx, "idx", `{"index":{"refresh_interval":"1s"}}`); err != nil {
		t.Fatalf("PutSettings error: %v", err)
	}
}

func TestClusterAndSnapshots(t *testing.T) {
	ctx := context.Background()
	c, ft := newFakeOSClient(t)

	ft.enqueue(200, `{"status":"green"}`)
	health, err := c.ClusterHealth(ctx)
	if err != nil || health == nil {
		t.Fatalf("ClusterHealth = %v, %v", health, err)
	}
	ft.enqueue(500, `{"error":{"reason":"down"},"status":500}`)
	if _, err := c.ClusterHealth(ctx); !errors.Is(err, ErrRequestFailed) {
		t.Fatalf("ClusterHealth(error resp) = %v", err)
	}

	ft.enqueue(200, `{"version":{"number":"2.0"}}`)
	info, err := c.ClusterInfo(ctx)
	if err != nil || info == nil {
		t.Fatalf("ClusterInfo = %v, %v", info, err)
	}

	ft.enqueue(200, okBody)
	if err := c.CreateSnapshot(ctx, "repo", "snap", `{"indices":["idx"]}`); err != nil {
		t.Fatalf("CreateSnapshot error: %v", err)
	}
	ft.enqueue(500, `{"error":{"reason":"x"},"status":500}`)
	if err := c.CreateSnapshot(ctx, "repo", "snap", ""); !errors.Is(err, ErrRequestFailed) {
		t.Fatalf("CreateSnapshot(error resp) = %v", err)
	}

	ft.enqueue(200, `{"snapshots":[]}`)
	snap, err := c.GetSnapshot(ctx, "repo", "snap")
	if err != nil || snap == nil {
		t.Fatalf("GetSnapshot = %v, %v", snap, err)
	}

	ft.enqueue(200, okBody)
	if err := c.DeleteSnapshot(ctx, "repo", "snap"); err != nil {
		t.Fatalf("DeleteSnapshot error: %v", err)
	}

	ft.enqueue(200, okBody)
	if err := c.CreateSnapshotRepository(ctx, "repo", `{"type":"fs"}`); err != nil {
		t.Fatalf("CreateSnapshotRepository error: %v", err)
	}

	ft.enqueue(200, okBody)
	if err := c.DeleteSnapshotRepository(ctx, "repo"); err != nil {
		t.Fatalf("DeleteSnapshotRepository error: %v", err)
	}

	ft.enqueue(200, `{"repo":{"type":"fs"}}`)
	r, err := c.GetSnapshotRepository(ctx, "repo")
	if err != nil || r == nil {
		t.Fatalf("GetSnapshotRepository = %v, %v", r, err)
	}
}

// ---------------------------------------------------------------------------
// Pure helpers
// ---------------------------------------------------------------------------

func TestParseErrorMessage(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(`{"error":{"reason":"r"},"status":400}`))}
	er, err := ParseErrorMessage(resp.Body)
	if err != nil || er.Status != 400 || er.Error.Reason != "r" {
		t.Fatalf("ParseErrorMessage = %+v, %v", er, err)
	}

	resp2 := &http.Response{Body: io.NopCloser(strings.NewReader("not-json"))}
	if _, err := ParseErrorMessage(resp2.Body); !errors.Is(err, ErrUnmarshalResponse) {
		t.Fatalf("ParseErrorMessage(bad json) = %v", err)
	}
}

func TestMergeOptions_Fake(t *testing.T) {
	// empty inputs produce an empty object
	out, err := MergeOptions("", "")
	if err != nil || out != "{}" {
		t.Fatalf("MergeOptions(empty) = %q, %v", out, err)
	}

	// plain objects are nested under their keys
	out, err = MergeOptions(`{"properties":{"a":{"type":"text"}}}`, `{"number_of_shards":1}`)
	if err != nil {
		t.Fatalf("MergeOptions error: %v", err)
	}
	if !strings.Contains(out, `"mappings"`) || !strings.Contains(out, `"settings"`) {
		t.Fatalf("MergeOptions = %q", out)
	}

	// pre-nested inputs are used verbatim
	out, err = MergeOptions(`{"mappings":{"properties":{}}}`, `{"settings":{}}`)
	if err != nil {
		t.Fatalf("MergeOptions(nested) error: %v", err)
	}
	if strings.Count(out, `"mappings"`) != 1 || strings.Count(out, `"settings"`) != 1 {
		t.Fatalf("MergeOptions(nested) = %q", out)
	}

	// malformed inputs fail
	if _, err := MergeOptions("{bad", ""); err == nil {
		t.Fatal("bad mapping must fail")
	}
	if _, err := MergeOptions("", "{bad"); err == nil {
		t.Fatal("bad settings must fail")
	}
}

func TestPartialFailureErrorFormat(t *testing.T) {
	e := &PartialFailureError{Total: 3, Failed: 1, FailedIDs: []string{"b"}}
	msg := e.Error()
	if !strings.Contains(msg, "1/3") || !strings.Contains(msg, "b") {
		t.Fatalf("PartialFailureError message = %q", msg)
	}
}

func TestMatchMarshalJSON(t *testing.T) {
	data, err := json.Marshal(Match{Field: "a", Value: "b"})
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}
	if string(data) != `{"a":"b"}` {
		t.Fatalf("Match JSON = %s", data)
	}
}

func TestTransportErrorPropagates(t *testing.T) {
	c, ft := newFakeOSClient(t)
	ft.err = errors.New("dial refused")
	if _, err := c.IndexExists(context.Background(), "idx"); err == nil {
		t.Fatal("transport failure must surface")
	}
}
