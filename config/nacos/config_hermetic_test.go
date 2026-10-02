package nacos

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"

	baseConfig "github.com/tx7do/go-wind-plugins/config"
)

// ---------------------------------------------------------------------------
// mock IConfigClient
// ---------------------------------------------------------------------------

// mockConfigClient is a hermetic fake of the nacos config client interface.
type mockConfigClient struct {
	mu sync.Mutex

	getConfigParam    *vo.ConfigParam
	getConfigContent  string
	getConfigErr      error
	getConfigCalls    int
	listenParam       *vo.ConfigParam
	listenErr         error
	listenCalls       int
	cancelParam       *vo.ConfigParam
	cancelListenCalls int
	onChange          func(namespace, group, dataId, data string)
	publishParam      *vo.ConfigParam
	searchParam       *vo.SearchConfigParam
	closeCalls        int
}

func (m *mockConfigClient) GetConfig(param vo.ConfigParam) (string, error) {
	m.mu.Lock()
	m.getConfigCalls++
	p := param
	m.getConfigParam = &p
	content, err := m.getConfigContent, m.getConfigErr
	m.mu.Unlock()
	return content, err
}

func (m *mockConfigClient) PublishConfig(param vo.ConfigParam) (bool, error) {
	m.mu.Lock()
	m.publishParam = &param
	m.mu.Unlock()
	return true, nil
}

func (m *mockConfigClient) DeleteConfig(param vo.ConfigParam) (bool, error) {
	return true, nil
}

func (m *mockConfigClient) ListenConfig(params vo.ConfigParam) error {
	m.mu.Lock()
	m.listenCalls++
	p := params
	m.listenParam = &p
	m.onChange = params.OnChange
	m.mu.Unlock()
	return m.listenErr
}

func (m *mockConfigClient) CancelListenConfig(params vo.ConfigParam) error {
	m.mu.Lock()
	m.cancelListenCalls++
	p := params
	m.cancelParam = &p
	m.mu.Unlock()
	return nil
}

func (m *mockConfigClient) SearchConfig(param vo.SearchConfigParam) (*model.ConfigPage, error) {
	m.mu.Lock()
	m.searchParam = &param
	m.mu.Unlock()
	return &model.ConfigPage{}, nil
}

func (m *mockConfigClient) CloseClient() {
	m.mu.Lock()
	m.closeCalls++
	m.mu.Unlock()
}

func (m *mockConfigClient) fireOnChange(namespace, group, dataId, data string) {
	m.mu.Lock()
	cb := m.onChange
	m.mu.Unlock()
	if cb != nil {
		cb(namespace, group, dataId, data)
	}
}

// compile-time: the mock must satisfy the client interface used by Config.
var _ config_client.IConfigClient = (*mockConfigClient)(nil)

// ---------------------------------------------------------------------------
// options
// ---------------------------------------------------------------------------

func TestOptions_Defaults(t *testing.T) {
	o := options{}
	if o.group != "" {
		t.Errorf("default group = %q, want empty", o.group)
	}
	if o.dataID != "" {
		t.Errorf("default dataID = %q, want empty", o.dataID)
	}
}

func TestOptions_Setters(t *testing.T) {
	o := options{}
	WithGroup("g1")(&o)
	if o.group != "g1" {
		t.Errorf("group = %q, want g1", o.group)
	}
	WithDataID("d1.yaml")(&o)
	if o.dataID != "d1.yaml" {
		t.Errorf("dataID = %q, want d1.yaml", o.dataID)
	}
}

func TestNew_Defaults(t *testing.T) {
	mc := &mockConfigClient{}
	c := New(mc)
	if c.client == nil {
		t.Fatal("New() did not store the client")
	}
	if c.opts.group != "" || c.opts.dataID != "" {
		t.Errorf("New() without options should keep zero options, got %+v", c.opts)
	}

	c = New(mc, WithGroup("g"), WithDataID("d"))
	if c.opts.group != "g" || c.opts.dataID != "d" {
		t.Errorf("New() with options = %+v, want group g dataID d", c.opts)
	}
}

// ---------------------------------------------------------------------------
// pure helpers
// ---------------------------------------------------------------------------

func TestResolveDataID(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		dataID string
		want   string
	}{
		{name: "key overrides default", key: "custom.yaml", dataID: "default.yaml", want: "custom.yaml"},
		{name: "empty key falls back", key: "", dataID: "default.yaml", want: "default.yaml"},
		{name: "both empty", key: "", dataID: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{opts: options{dataID: tt.dataID}}
			if got := c.resolveDataID(tt.key); got != tt.want {
				t.Errorf("resolveDataID(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestGetConfigKey(t *testing.T) {
	tests := []struct {
		key          string
		useBackslash bool
		want         string
	}{
		{"a.b.c", true, "a/b/c"},
		{"a.b.c", false, "a.b.c"},
		{"abc", true, "abc"},
		{"", true, ""},
	}
	for _, tt := range tests {
		if got := getConfigKey(tt.key, tt.useBackslash); got != tt.want {
			t.Errorf("getConfigKey(%q, %v) = %q, want %q", tt.key, tt.useBackslash, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Load
// ---------------------------------------------------------------------------

func TestConfig_Load_Success(t *testing.T) {
	mc := &mockConfigClient{getConfigContent: "hello: world"}
	c := New(mc, WithGroup("g1"), WithDataID("bootstrap.yaml"))

	data, err := c.Load(context.Background(), "")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if string(data) != "hello: world" {
		t.Errorf("Load() = %q, want %q", string(data), "hello: world")
	}

	if mc.getConfigParam == nil {
		t.Fatal("GetConfig was not called")
	}
	if mc.getConfigParam.DataId != "bootstrap.yaml" {
		t.Errorf("GetConfig DataId = %q, want bootstrap.yaml", mc.getConfigParam.DataId)
	}
	if mc.getConfigParam.Group != "g1" {
		t.Errorf("GetConfig Group = %q, want g1", mc.getConfigParam.Group)
	}
}

func TestConfig_Load_KeyOverridesDataID(t *testing.T) {
	mc := &mockConfigClient{getConfigContent: "x"}
	c := New(mc, WithGroup("g"), WithDataID("default.yaml"))

	if _, err := c.Load(context.Background(), "override.yaml"); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if mc.getConfigParam.DataId != "override.yaml" {
		t.Errorf("GetConfig DataId = %q, want override.yaml", mc.getConfigParam.DataId)
	}
}

func TestConfig_Load_Error(t *testing.T) {
	wantErr := errors.New("boom")
	mc := &mockConfigClient{getConfigErr: wantErr}
	c := New(mc)

	data, err := c.Load(context.Background(), "k")
	if err == nil {
		t.Fatal("Load() expected error")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("Load() error = %v, want %v", err, wantErr)
	}
	if data != nil {
		t.Errorf("Load() data = %v, want nil", data)
	}
}

// ---------------------------------------------------------------------------
// WatchValue
// ---------------------------------------------------------------------------

func TestConfig_WatchValue_DeliversChange(t *testing.T) {
	mc := &mockConfigClient{}
	c := New(mc, WithGroup("g1"), WithDataID("app.yaml"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := c.WatchValue(ctx, "")
	if err != nil {
		t.Fatalf("WatchValue() error = %v", err)
	}

	if mc.listenParam == nil {
		t.Fatal("ListenConfig was not called")
	}
	if mc.listenParam.DataId != "app.yaml" || mc.listenParam.Group != "g1" {
		t.Errorf("ListenConfig param = %+v, want dataID app.yaml group g1", *mc.listenParam)
	}

	// Fire a change with the exact dataID/group: value must be delivered.
	mc.fireOnChange("public", "g1", "app.yaml", "v2")

	select {
	case got := <-ch:
		if string(got) != "v2" {
			t.Errorf("watch got %q, want %q", string(got), "v2")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("watch value not delivered within 2s")
	}

	// A change for a different dataID must be filtered out.
	mc.fireOnChange("public", "g1", "other.yaml", "v3")
	// A change for a different group must be filtered out too.
	mc.fireOnChange("public", "other-group", "app.yaml", "v4")

	select {
	case got := <-ch:
		t.Errorf("filtered change unexpectedly delivered: %q", string(got))
	case <-time.After(150 * time.Millisecond):
		// expected: nothing arrives
	}
}

func TestConfig_WatchValue_CancelClosesChannel(t *testing.T) {
	mc := &mockConfigClient{}
	c := New(mc, WithGroup("g"), WithDataID("d.yaml"))

	ctx, cancel := context.WithCancel(context.Background())

	ch, err := c.WatchValue(ctx, "")
	if err != nil {
		t.Fatalf("WatchValue() error = %v", err)
	}

	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			t.Error("channel should be closed after ctx cancel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("channel not closed within 2s after cancel")
	}

	// The unlisten must have been issued with the same coordinates.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mc.mu.Lock()
		calls, param := mc.cancelListenCalls, mc.cancelParam
		mc.mu.Unlock()
		if calls > 0 {
			if param.DataId != "d.yaml" || param.Group != "g" {
				t.Errorf("CancelListenConfig param = %+v, want dataID d.yaml group g", *param)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("CancelListenConfig was not called after ctx cancel")
}

func TestConfig_WatchValue_KeyOverride(t *testing.T) {
	mc := &mockConfigClient{}
	c := New(mc, WithGroup("g"), WithDataID("default.yaml"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := c.WatchValue(ctx, "explicit.yaml"); err != nil {
		t.Fatalf("WatchValue() error = %v", err)
	}
	if mc.listenParam.DataId != "explicit.yaml" {
		t.Errorf("ListenConfig DataId = %q, want explicit.yaml", mc.listenParam.DataId)
	}
}

func TestConfig_WatchValue_ListenError(t *testing.T) {
	wantErr := errors.New("listen refused")
	mc := &mockConfigClient{listenErr: wantErr}
	c := New(mc)

	_, err := c.WatchValue(context.Background(), "")
	if err == nil {
		t.Fatal("WatchValue() expected error")
	}
	if !strings.Contains(err.Error(), "listen refused") {
		t.Errorf("WatchValue() error = %v, want it to wrap %v", err, wantErr)
	}
}

// ---------------------------------------------------------------------------
// interface compliance
// ---------------------------------------------------------------------------

func TestConfig_ImplementsReaderAndWatcher(t *testing.T) {
	var _ baseConfig.Reader = (*Config)(nil)
	var _ baseConfig.ValueWatcher = (*Config)(nil)
	_ = New(&mockConfigClient{})
}
