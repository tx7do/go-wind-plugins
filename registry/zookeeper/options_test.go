package zookeeper

import (
	"encoding/json"
	"testing"

	wind "github.com/tx7do/go-wind"
)

// The zk.Conn type dials the server inside zk.Connect and exposes no
// injectable internals, so only the connection-free surface (options and the
// JSON helpers) can be exercised hermetically here. The integration tests in
// register_test.go cover the conn-backed paths.

func TestNew_Defaults(t *testing.T) {
	// New must not touch the (possibly nil) connection.
	r := New(nil)

	if r.opts.namespace != "/microservices" {
		t.Errorf("namespace = %q, want /microservices", r.opts.namespace)
	}
	if r.opts.user != "" || r.opts.password != "" {
		t.Errorf("user/password = %q/%q, want empty by default", r.opts.user, r.opts.password)
	}
	if r.conn != nil {
		t.Error("conn = non-nil, want the provided nil connection")
	}
}

func TestWithRootPath(t *testing.T) {
	r := New(nil, WithRootPath("/services"))
	if r.opts.namespace != "/services" {
		t.Errorf("namespace = %q, want /services", r.opts.namespace)
	}
}

func TestWithDigestACL(t *testing.T) {
	r := New(nil, WithDigestACL("user", "secret"))
	if r.opts.user != "user" {
		t.Errorf("user = %q, want user", r.opts.user)
	}
	if r.opts.password != "secret" {
		t.Errorf("password = %q, want secret", r.opts.password)
	}
}

func TestOptions_LastWins(t *testing.T) {
	r := New(nil, WithRootPath("/a"), WithRootPath("/b"))
	if r.opts.namespace != "/b" {
		t.Errorf("namespace = %q, want /b (last option wins)", r.opts.namespace)
	}
}

func TestMarshalUnmarshal_RoundTrip(t *testing.T) {
	in := &wind.Instance{
		ID:        "1",
		Name:      "hello",
		Version:   "v1.0.0",
		Endpoints: []string{"127.0.0.1:8080"},
		Metadata:  map[string]string{"env": "test"},
	}

	data, err := marshal(in)
	if err != nil {
		t.Fatalf("marshal() error = %v", err)
	}

	out, err := unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal() error = %v", err)
	}

	if out.ID != in.ID || out.Name != in.Name || out.Version != in.Version {
		t.Errorf("round trip = %+v, want %+v", out, in)
	}
	if len(out.Endpoints) != 1 || out.Endpoints[0] != "127.0.0.1:8080" {
		t.Errorf("Endpoints = %v, want [127.0.0.1:8080]", out.Endpoints)
	}
	if out.Metadata["env"] != "test" {
		t.Errorf("Metadata = %v, want env=test", out.Metadata)
	}
}

func TestMarshal_ProducesJSON(t *testing.T) {
	data, err := marshal(&wind.Instance{ID: "7"})
	if err != nil {
		t.Fatalf("marshal() error = %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("marshal() output is not valid JSON: %v", err)
	}
	if raw["id"] != "7" {
		t.Errorf("marshal() = %s, want an id field of 7", data)
	}
}

func TestUnmarshal_InvalidJSON(t *testing.T) {
	if _, err := unmarshal([]byte("not json")); err == nil {
		t.Error("unmarshal() with invalid JSON should fail")
	}
}

// Empty input is not valid JSON, so unmarshal must report an error rather
// than return a nil instance silently.
func TestUnmarshal_Empty(t *testing.T) {
	if _, err := unmarshal(nil); err == nil {
		t.Error("unmarshal(nil) should fail with an empty-input error")
	}
}
