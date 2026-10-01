package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/executor"
)

// stubResolver is a ResolverRoot implementation returning canned results.
type stubResolver struct {
	hg  *Hygrothermograph
	err error

	queryCalled bool
}

func (s *stubResolver) Query() QueryResolver { return s }

func (s *stubResolver) Hygrothermograph(ctx context.Context) (*Hygrothermograph, error) {
	s.queryCalled = true
	return s.hg, s.err
}

var _ ResolverRoot = (*stubResolver)(nil)
var _ QueryResolver = (*stubResolver)(nil)

func newTestSchema(res ResolverRoot) graphql.ExecutableSchema {
	return NewExecutableSchema(Config{
		Schema:    parsedSchema,
		Resolvers: res,
	})
}

// execute runs a GraphQL document against the schema and returns the response.
func execute(t *testing.T, schema graphql.ExecutableSchema, query string) *graphql.Response {
	t.Helper()

	exec := executor.New(schema)
	ctx := graphql.StartOperationTrace(context.Background())
	params := &graphql.RawParams{Query: query}

	opCtx, errList := exec.CreateOperationContext(ctx, params)
	if len(errList) != 0 {
		t.Fatalf("CreateOperationContext returned errors: %v", errList)
	}

	handler, execCtx := exec.DispatchOperation(ctx, opCtx)
	resp := handler(execCtx)
	if resp == nil {
		t.Fatal("query handler returned a nil response")
	}
	return resp
}

// ---------------------------------------------------------------------------
// Models
// ---------------------------------------------------------------------------

func TestHygrothermographModel(t *testing.T) {
	hg := Hygrothermograph{Humidity: 55.5, Temperature: 23.75}

	if hg.Humidity != 55.5 {
		t.Errorf("Humidity = %v, want 55.5", hg.Humidity)
	}
	if hg.Temperature != 23.75 {
		t.Errorf("Temperature = %v, want 23.75", hg.Temperature)
	}
}

func TestQueryModel(t *testing.T) {
	// The generated Query type is an empty carrier struct; construction
	// must simply work.
	q := Query{}
	_ = q
}

// ---------------------------------------------------------------------------
// Schema construction
// ---------------------------------------------------------------------------

func TestNewExecutableSchema(t *testing.T) {
	schema := newTestSchema(&stubResolver{})
	if schema == nil {
		t.Fatal("NewExecutableSchema returned nil")
	}

	// The parsed schema must expose the schema.graphql types.
	s := schema.Schema()
	if s == nil {
		t.Fatal("Schema() returned nil")
	}
	for _, typeName := range []string{"Query", "Hygrothermograph"} {
		if _, ok := s.Types[typeName]; !ok {
			t.Errorf("schema should define type %q", typeName)
		}
	}
}

// ---------------------------------------------------------------------------
// Query execution
// ---------------------------------------------------------------------------

func TestQueryHygrothermograph(t *testing.T) {
	res := &stubResolver{hg: &Hygrothermograph{Humidity: 55.5, Temperature: 23.75}}
	schema := newTestSchema(res)

	resp := execute(t, schema, "{ hygrothermograph { humidity temperature } }")

	if len(resp.Errors) != 0 {
		t.Fatalf("unexpected response errors: %v", resp.Errors)
	}
	if !res.queryCalled {
		t.Error("the Query.hygrothermograph resolver should have been called")
	}

	var out struct {
		Hygrothermograph struct {
			Humidity    float64 `json:"humidity"`
			Temperature float64 `json:"temperature"`
		} `json:"hygrothermograph"`
	}
	if err := json.Unmarshal(resp.Data, &out); err != nil {
		t.Fatalf("failed to decode response data %s: %v", resp.Data, err)
	}
	if out.Hygrothermograph.Humidity != 55.5 {
		t.Errorf("humidity = %v, want 55.5", out.Hygrothermograph.Humidity)
	}
	if out.Hygrothermograph.Temperature != 23.75 {
		t.Errorf("temperature = %v, want 23.75", out.Hygrothermograph.Temperature)
	}
}

func TestQueryHygrothermograph_PartialSelection(t *testing.T) {
	res := &stubResolver{hg: &Hygrothermograph{Humidity: 10, Temperature: 20}}
	schema := newTestSchema(res)

	// Only one field selected: the other must not appear in the response.
	resp := execute(t, schema, "{ hygrothermograph { temperature } }")

	if len(resp.Errors) != 0 {
		t.Fatalf("unexpected response errors: %v", resp.Errors)
	}
	if !res.queryCalled {
		t.Error("the Query.hygrothermograph resolver should have been called")
	}

	var out struct {
		Hygrothermograph struct {
			Humidity    *float64 `json:"humidity"`
			Temperature *float64 `json:"temperature"`
		} `json:"hygrothermograph"`
	}
	if err := json.Unmarshal(resp.Data, &out); err != nil {
		t.Fatalf("failed to decode response data %s: %v", resp.Data, err)
	}
	if out.Hygrothermograph.Humidity != nil {
		t.Errorf("humidity = %v, want absent", *out.Hygrothermograph.Humidity)
	}
	if out.Hygrothermograph.Temperature == nil || *out.Hygrothermograph.Temperature != 20 {
		t.Errorf("temperature = %v, want 20", out.Hygrothermograph.Temperature)
	}
}

func TestQueryHygrothermograph_ResolverError(t *testing.T) {
	res := &stubResolver{err: errors.New("sensor offline")}
	schema := newTestSchema(res)

	resp := execute(t, schema, "{ hygrothermograph { humidity } }")

	if len(resp.Errors) == 0 {
		t.Fatal("expected response errors from a failing resolver")
	}
	found := false
	for _, e := range resp.Errors {
		if strings.Contains(e.Message, "sensor offline") {
			found = true
		}
	}
	if !found {
		t.Errorf("error message should mention the resolver error, got %v", resp.Errors)
	}
	if resp.Data == nil {
		t.Errorf("data = %s, want non-nil on error", resp.Data)
	}
}

func TestMutationRejected(t *testing.T) {
	// The schema only defines Query; a mutation must be rejected at validation.
	res := &stubResolver{hg: &Hygrothermograph{}}
	schema := newTestSchema(res)

	exec := executor.New(schema)
	ctx := graphql.StartOperationTrace(context.Background())

	_, errList := exec.CreateOperationContext(ctx, &graphql.RawParams{
		Query: "mutation { __typename }",
	})
	if len(errList) == 0 {
		t.Fatal("expected the mutation to be rejected")
	}
	if res.queryCalled {
		t.Error("the query resolver must not run for a mutation")
	}
}

// ---------------------------------------------------------------------------
// Complexity
// ---------------------------------------------------------------------------

// newComplexitySchema wires a ComplexityRoot where every field costs child+1.
func newComplexitySchema() graphql.ExecutableSchema {
	c := ComplexityRoot{}
	c.Hygrothermograph.Humidity = func(childComplexity int) int { return childComplexity + 1 }
	c.Hygrothermograph.Temperature = func(childComplexity int) int { return childComplexity + 1 }
	c.Query.Hygrothermograph = func(childComplexity int) int { return childComplexity + 1 }
	return NewExecutableSchema(Config{Schema: parsedSchema, Complexity: c})
}

func TestComplexity_LeafFields(t *testing.T) {
	schema := newComplexitySchema()

	got, ok := schema.Complexity(context.Background(), "Hygrothermograph", "humidity", 0, nil)
	if !ok {
		t.Fatal("Complexity should report Hygrothermograph.humidity")
	}
	if got != 1 {
		t.Errorf("Complexity(Hygrothermograph.humidity) = %d, want 1", got)
	}
}

func TestComplexity_QueryFieldIncludesChild(t *testing.T) {
	schema := newComplexitySchema()

	got, ok := schema.Complexity(context.Background(), "Query", "hygrothermograph", 3, nil)
	if !ok {
		t.Fatal("Complexity should report Query.hygrothermograph")
	}
	if got != 4 {
		t.Errorf("Complexity(Query.hygrothermograph, child=3) = %d, want 4", got)
	}
}

func TestComplexity_UnknownField(t *testing.T) {
	schema := newComplexitySchema()

	got, ok := schema.Complexity(context.Background(), "Query", "nonexistent", 1, nil)
	if ok {
		t.Error("Complexity should not report unknown fields")
	}
	if got != 0 {
		t.Errorf("Complexity of an unknown field = %d, want 0", got)
	}
}
