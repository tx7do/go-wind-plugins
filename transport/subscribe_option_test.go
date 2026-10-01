package transport

import (
	"context"
	"errors"
	"testing"

	"github.com/tx7do/go-wind-plugins/broker"
)

// ---------------------------------------------------------------------------
// SubscribeOption
// ---------------------------------------------------------------------------

func TestSubscribeOption_Fields(t *testing.T) {
	handler := func(ctx context.Context, evt broker.Event) error { return nil }
	binder := func() any { return new(struct{}) }
	opts := []broker.SubscribeOption{}

	so := &SubscribeOption{
		Handler:          handler,
		Binder:           binder,
		SubscribeOptions: opts,
	}

	if so.Handler == nil {
		t.Error("Handler should be set")
	}
	if so.Binder == nil {
		t.Error("Binder should be set")
	}
	if len(so.SubscribeOptions) != 0 {
		t.Errorf("SubscribeOptions length = %d, want 0", len(so.SubscribeOptions))
	}
}

func TestSubscribeOption_HandlerInvocation(t *testing.T) {
	called := false
	so := &SubscribeOption{
		Handler: func(ctx context.Context, evt broker.Event) error {
			called = true
			return nil
		},
	}

	if err := so.Handler(context.Background(), nil); err != nil {
		t.Errorf("Handler() error = %v, want nil", err)
	}
	if !called {
		t.Error("Handler should have been invoked")
	}
}

func TestSubscribeOption_HandlerError(t *testing.T) {
	wantErr := errors.New("handler failed")
	so := &SubscribeOption{
		Handler: func(ctx context.Context, evt broker.Event) error { return wantErr },
	}

	if err := so.Handler(context.Background(), nil); !errors.Is(err, wantErr) {
		t.Errorf("Handler() error = %v, want %v", err, wantErr)
	}
}

func TestSubscribeOption_BinderInvocation(t *testing.T) {
	so := &SubscribeOption{
		Binder: func() any { return "bound-value" },
	}

	got := so.Binder()
	if got != "bound-value" {
		t.Errorf("Binder() = %v, want %q", got, "bound-value")
	}
}

func TestSubscribeOption_ZeroValue(t *testing.T) {
	var so SubscribeOption

	if so.Handler != nil {
		t.Error("zero value Handler should be nil")
	}
	if so.Binder != nil {
		t.Error("zero value Binder should be nil")
	}
	if so.SubscribeOptions != nil {
		t.Error("zero value SubscribeOptions should be nil")
	}
}

// ---------------------------------------------------------------------------
// SubscribeOptionMap
// ---------------------------------------------------------------------------

func TestSubscribeOptionMap_InsertAndLookup(t *testing.T) {
	m := make(SubscribeOptionMap)
	so := &SubscribeOption{}
	m["topic.a"] = so

	got, ok := m["topic.a"]
	if !ok {
		t.Fatal("expected topic.a to be present in the map")
	}
	if got != so {
		t.Error("lookup should return the stored SubscribeOption pointer")
	}
}

func TestSubscribeOptionMap_MissingKey(t *testing.T) {
	m := make(SubscribeOptionMap)

	got, ok := m["nonexistent"]
	if ok {
		t.Error("lookup of a missing key should report ok = false")
	}
	if got != nil {
		t.Errorf("lookup of a missing key = %v, want nil", got)
	}
}

func TestSubscribeOptionMap_MultipleTopics(t *testing.T) {
	m := SubscribeOptionMap{
		"topic.a": {Handler: nil},
		"topic.b": {Handler: nil},
		"topic.c": {Handler: nil},
	}

	if len(m) != 3 {
		t.Errorf("map length = %d, want 3", len(m))
	}
	for _, topic := range []string{"topic.a", "topic.b", "topic.c"} {
		if _, ok := m[topic]; !ok {
			t.Errorf("expected topic %q to be present", topic)
		}
	}
}

func TestSubscribeOptionMap_Overwrite(t *testing.T) {
	first := &SubscribeOption{}
	second := &SubscribeOption{}

	m := SubscribeOptionMap{"topic.a": first}
	m["topic.a"] = second

	if m["topic.a"] != second {
		t.Error("overwrite should replace the stored SubscribeOption")
	}
}

func TestSubscribeOptionMap_Delete(t *testing.T) {
	m := SubscribeOptionMap{"topic.a": {}}
	delete(m, "topic.a")

	if len(m) != 0 {
		t.Errorf("map length after delete = %d, want 0", len(m))
	}
}

func TestSubscribeOptionMap_NilMap(t *testing.T) {
	var m SubscribeOptionMap

	if len(m) != 0 {
		t.Errorf("nil map length = %d, want 0", len(m))
	}
	if _, ok := m["topic.a"]; ok {
		t.Error("lookup in a nil map should report ok = false")
	}
}
