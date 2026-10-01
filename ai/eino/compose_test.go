package eino

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	jsonschema "github.com/eino-contrib/jsonschema"
)

// ---------------------------------------------------------------------------
// Lambda wrappers through a compiled Chain
// ---------------------------------------------------------------------------

func TestInvokableLambda_ChainInvoke(t *testing.T) {
	chain := NewChain[string, string]()
	chain.AppendLambda(InvokableLambda(func(ctx context.Context, input string) (string, error) {
		return strings.ToUpper(input), nil
	}))

	runnable, err := CompileChain(context.Background(), chain)
	if err != nil {
		t.Fatalf("CompileChain returned error: %v", err)
	}

	got, err := runnable.Invoke(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
	if got != "HELLO" {
		t.Errorf("Invoke = %q, want %q", got, "HELLO")
	}
}

func TestStreamableLambda_ChainStream(t *testing.T) {
	chain := NewChain[string, string]()
	chain.AppendLambda(StreamableLambda(func(ctx context.Context, input string) (*schema.StreamReader[string], error) {
		sr, sw := schema.Pipe[string](2)
		sw.Send(input, nil)
		sw.Send("-tail", nil)
		sw.Close()
		return sr, nil
	}))

	runnable, err := CompileChain(context.Background(), chain)
	if err != nil {
		t.Fatalf("CompileChain returned error: %v", err)
	}

	sr, err := runnable.Stream(context.Background(), "head")
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	defer sr.Close()

	var sb strings.Builder
	for {
		chunk, err := sr.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Recv returned error: %v", err)
		}
		sb.WriteString(chunk)
	}
	if got := sb.String(); got != "head-tail" {
		t.Errorf("streamed output = %q, want %q", got, "head-tail")
	}
}

func TestToList_ChainInvoke(t *testing.T) {
	chain := NewChain[string, []string]()
	chain.AppendLambda(ToList[string]())

	runnable, err := CompileChain(context.Background(), chain)
	if err != nil {
		t.Fatalf("CompileChain returned error: %v", err)
	}

	got, err := runnable.Invoke(context.Background(), "item")
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
	if len(got) != 1 || got[0] != "item" {
		t.Errorf("Invoke = %v, want [item]", got)
	}
}

func TestAnyLambda_RequiresNonNull(t *testing.T) {
	// AnyLambda with all-nil modes must fail.
	if _, err := AnyLambda[string, string, struct{}](nil, nil, nil, nil); err == nil {
		t.Error("AnyLambda with no modes should return an error")
	}

	// A single invoke mode is sufficient.
	lambda, err := AnyLambda[string, string, compose.Option](
		func(ctx context.Context, input string, opts ...compose.Option) (string, error) {
			return strings.ToUpper(input), nil
		}, nil, nil, nil)
	if err != nil {
		t.Fatalf("AnyLambda returned error: %v", err)
	}
	if lambda == nil {
		t.Fatal("AnyLambda returned nil lambda")
	}
}

func TestCollectableLambda(t *testing.T) {
	lambda := CollectableLambda(func(ctx context.Context, input *schema.StreamReader[string]) (string, error) {
		defer input.Close()
		var sb strings.Builder
		for {
			chunk, err := input.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return "", err
			}
			sb.WriteString(chunk)
		}
		return sb.String(), nil
	})
	if lambda == nil {
		t.Fatal("CollectableLambda returned nil")
	}
}

func TestNewGraphConstruction(t *testing.T) {
	g := NewGraph[string, string]()
	if g == nil {
		t.Fatal("NewGraph returned nil")
	}
	if err := g.AddLambdaNode("upper", InvokableLambda(func(ctx context.Context, input string) (string, error) {
		return strings.ToUpper(input), nil
	})); err != nil {
		t.Fatalf("AddLambdaNode returned error: %v", err)
	}
	if err := g.AddEdge(START, "upper"); err != nil {
		t.Fatalf("AddEdge returned error: %v", err)
	}
	if err := g.AddEdge("upper", END); err != nil {
		t.Fatalf("AddEdge returned error: %v", err)
	}

	runnable, err := g.Compile(context.Background())
	if err != nil {
		t.Fatalf("Compile returned error: %v", err)
	}
	got, err := runnable.Invoke(context.Background(), "go")
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
	if got != "GO" {
		t.Errorf("Invoke = %q, want %q", got, "GO")
	}
}

// ---------------------------------------------------------------------------
// Tool info helpers
// ---------------------------------------------------------------------------

func TestNewToolInfo(t *testing.T) {
	params := NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
		"text": NewParameterInfo(schema.String, "the text", true, WithEnum([]string{"a", "b"})),
	})
	info := NewToolInfo("echo", "echoes the text", params)

	if info == nil {
		t.Fatal("NewToolInfo returned nil")
	}
	if info.Name != "echo" {
		t.Errorf("Name = %q, want %q", info.Name, "echo")
	}
	if info.Desc != "echoes the text" {
		t.Errorf("Desc = %q, want %q", info.Desc, "echoes the text")
	}
	if info.ParamsOneOf == nil {
		t.Error("ParamsOneOf should not be nil")
	}
}

func TestNewToolInfo_NilParams(t *testing.T) {
	info := NewToolInfo("noargs", "takes no arguments", nil)
	if info.ParamsOneOf != nil {
		t.Error("ParamsOneOf should be nil when no params are passed")
	}
}

func TestNewParameterInfo(t *testing.T) {
	pi := NewParameterInfo(schema.Integer, "how many", true)
	if pi.Type != schema.Integer {
		t.Errorf("Type = %v, want %v", pi.Type, schema.Integer)
	}
	if pi.Desc != "how many" {
		t.Errorf("Desc = %q, want %q", pi.Desc, "how many")
	}
	if !pi.Required {
		t.Error("Required = false, want true")
	}
	if pi.Enum != nil || pi.SubParams != nil || pi.ElemInfo != nil {
		t.Error("default ParameterInfo should have no Enum/SubParams/ElemInfo")
	}
}

func TestParameterInfoOptions(t *testing.T) {
	pi := NewParameterInfo(schema.String, "mode", false,
		WithEnum([]string{"fast", "slow"}),
	)
	if len(pi.Enum) != 2 {
		t.Errorf("Enum = %v, want 2 values", pi.Enum)
	}

	sub := map[string]*schema.ParameterInfo{
		"name": NewParameterInfo(schema.String, "name", true),
	}
	pi2 := NewParameterInfo(schema.Object, "nested", false, WithSubParams(sub))
	if len(pi2.SubParams) != 1 {
		t.Errorf("SubParams = %v, want 1 entry", pi2.SubParams)
	}

	pi3 := NewParameterInfo(schema.Array, "list", false, WithElemInfo(NewParameterInfo(schema.Number, "element", false)))
	if pi3.ElemInfo == nil {
		t.Fatal("ElemInfo should not be nil")
	}
	if pi3.ElemInfo.Type != schema.Number {
		t.Errorf("ElemInfo.Type = %v, want %v", pi3.ElemInfo.Type, schema.Number)
	}
}

func TestNewParamsOneOfByParams(t *testing.T) {
	po := NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
		"query": NewParameterInfo(schema.String, "search query", true),
	})
	if po == nil {
		t.Fatal("NewParamsOneOfByParams returned nil")
	}

	// The params view should be convertible back to JSON schema.
	got, err := po.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema returned error: %v", err)
	}
	if got == nil {
		t.Fatal("ToJSONSchema returned nil")
	}
}

func TestNewParamsOneOfByJSONSchema(t *testing.T) {
	po := NewParamsOneOfByJSONSchema(&jsonschema.Schema{})
	if po == nil {
		t.Fatal("NewParamsOneOfByJSONSchema returned nil")
	}
}

// ---------------------------------------------------------------------------
// NewToolNode with a mock tool
// ---------------------------------------------------------------------------

// mockTool is a minimal InvokableTool for exercising ToolsNode.
type mockTool struct {
	gotArgs string
}

var _ tool.BaseTool = &mockTool{}
var _ tool.InvokableTool = &mockTool{}

func (m mockTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return NewToolInfo("echo", "echoes its arguments", nil), nil
}

func (m *mockTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	m.gotArgs = argumentsInJSON
	return "echo:" + argumentsInJSON, nil
}

func TestNewToolNode(t *testing.T) {
	node, err := NewToolNode(context.Background(), &ToolsNodeConfig{
		Tools: []tool.BaseTool{&mockTool{}},
	})
	if err != nil {
		t.Fatalf("NewToolNode returned error: %v", err)
	}
	if node == nil {
		t.Fatal("NewToolNode returned nil node")
	}
}

func TestToolNodeInvoke(t *testing.T) {
	tl := &mockTool{}
	node, err := NewToolNode(context.Background(), &ToolsNodeConfig{Tools: []tool.BaseTool{tl}})
	if err != nil {
		t.Fatalf("NewToolNode returned error: %v", err)
	}

	msg := &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{{
			ID:   "call-1",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "echo",
				Arguments: `{"text":"hi"}`,
			},
		}},
	}

	out, err := node.Invoke(context.Background(), msg, WithToolList(tl))
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("len(out) = %d, want 1", len(out))
	}
	if out[0].Role != schema.Tool {
		t.Errorf("out[0].Role = %q, want %q", out[0].Role, schema.Tool)
	}
	if out[0].ToolCallID != "call-1" {
		t.Errorf("out[0].ToolCallID = %q, want %q", out[0].ToolCallID, "call-1")
	}
	if out[0].Content != `echo:{"text":"hi"}` {
		t.Errorf("out[0].Content = %q, want %q", out[0].Content, `echo:{"text":"hi"}`)
	}
	if tl.gotArgs != `{"text":"hi"}` {
		t.Errorf("tool received arguments %q, want %q", tl.gotArgs, `{"text":"hi"}`)
	}
}

func TestToolNodeInvoke_UnknownTool(t *testing.T) {
	node, err := NewToolNode(context.Background(), &ToolsNodeConfig{Tools: []tool.BaseTool{&mockTool{}}})
	if err != nil {
		t.Fatalf("NewToolNode returned error: %v", err)
	}

	msg := &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{{
			ID:       "call-2",
			Function: schema.FunctionCall{Name: "missing", Arguments: "{}"},
		}},
	}

	if _, err := node.Invoke(context.Background(), msg); err == nil {
		t.Error("Invoke with an unknown tool name should return an error")
	}
}

func TestWithToolOption(t *testing.T) {
	opt := WithToolOption(tool.WrapImplSpecificOptFn(func(cfg *map[string]any) {}))
	if opt == nil {
		t.Error("WithToolOption should return a non-nil option")
	}
}
