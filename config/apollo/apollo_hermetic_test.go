package apollo

import (
	"container/list"
	"context"
	stdjson "encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/apolloconfig/agollo/v4/agcache"
	apolloconfig "github.com/apolloconfig/agollo/v4/env/config"
	"github.com/apolloconfig/agollo/v4/storage"
)

// ---------------------------------------------------------------------------
// mocks
// ---------------------------------------------------------------------------

// mockCache is a hermetic fake of agcache.CacheInterface backed by a map.
type mockCache struct {
	data map[string]any
	get  func(key string) (any, error)
}

func (m *mockCache) Set(key string, value any, expireSeconds int) error {
	m.data[key] = value
	return nil
}

func (m *mockCache) EntryCount() int64 { return int64(len(m.data)) }

func (m *mockCache) Get(key string) (any, error) {
	if m.get != nil {
		return m.get(key)
	}
	v, ok := m.data[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return v, nil
}

func (m *mockCache) Del(key string) bool {
	_, ok := m.data[key]
	delete(m.data, key)
	return ok
}

func (m *mockCache) Range(f func(key, value any) bool) {
	for k, v := range m.data {
		if !f(k, v) {
			return
		}
	}
}

func (m *mockCache) Clear() { m.data = map[string]any{} }

// mockClient is a hermetic fake of agollo.Client.
type mockClient struct {
	cache        *mockCache
	added        []storage.ChangeListener
	removed      []storage.ChangeListener
	namespace    string
	getConfigNS  string
	getConfigRet *storage.Config
}

func (m *mockClient) GetConfig(namespace string) *storage.Config {
	m.getConfigNS = namespace
	return m.getConfigRet
}

func (m *mockClient) GetConfigAndInit(namespace string) *storage.Config { return nil }

func (m *mockClient) GetConfigCache(namespace string) agcache.CacheInterface {
	m.namespace = namespace
	return m.cache
}

func (m *mockClient) GetDefaultConfigCache() agcache.CacheInterface { return m.cache }

func (m *mockClient) GetApolloConfigCache() agcache.CacheInterface { return m.cache }

func (m *mockClient) GetValue(key string) string                             { return "" }
func (m *mockClient) GetStringValue(key string, defaultValue string) string  { return defaultValue }
func (m *mockClient) GetIntValue(key string, defaultValue int) int           { return defaultValue }
func (m *mockClient) GetFloatValue(key string, defaultValue float64) float64 { return defaultValue }
func (m *mockClient) GetBoolValue(key string, defaultValue bool) bool        { return defaultValue }
func (m *mockClient) GetStringSliceValue(key string, defaultValue []string) []string {
	return defaultValue
}
func (m *mockClient) GetIntSliceValue(key string, defaultValue []int) []int { return defaultValue }

func (m *mockClient) AddChangeListener(listener storage.ChangeListener) {
	m.added = append(m.added, listener)
}

func (m *mockClient) RemoveChangeListener(listener storage.ChangeListener) {
	m.removed = append(m.removed, listener)
}

func (m *mockClient) GetChangeListeners() *list.List { return list.New() }

func (m *mockClient) UseEventDispatch() {}

func (m *mockClient) Close() {}

// ---------------------------------------------------------------------------
// options
// ---------------------------------------------------------------------------

func TestOptions_Setters(t *testing.T) {
	o := options{}
	WithAppID("app-1")(&o)
	WithCluster("c1")(&o)
	WithEndpoint("http://apollo:8080")(&o)
	WithSecret("s3cret")(&o)
	WithNamespace("ns1.yaml,ns2")(&o)
	WithBackupPath("/backup")(&o)

	if o.appid != "app-1" {
		t.Errorf("appid = %q, want app-1", o.appid)
	}
	if o.cluster != "c1" {
		t.Errorf("cluster = %q, want c1", o.cluster)
	}
	if o.endpoint != "http://apollo:8080" {
		t.Errorf("endpoint = %q, want http://apollo:8080", o.endpoint)
	}
	if o.secret != "s3cret" {
		t.Errorf("secret = %q, want s3cret", o.secret)
	}
	if o.namespace != "ns1.yaml,ns2" {
		t.Errorf("namespace = %q, want ns1.yaml,ns2", o.namespace)
	}
	if o.backupPath != "/backup" {
		t.Errorf("backupPath = %q, want /backup", o.backupPath)
	}
	if o.isBackupConfig {
		t.Error("isBackupConfig should still be false without backup options")
	}
}

func TestOptions_BackupToggle(t *testing.T) {
	o := options{}
	WithEnableBackup()(&o)
	if !o.isBackupConfig {
		t.Error("WithEnableBackup must set isBackupConfig")
	}
	WithDisableBackup()(&o)
	if o.isBackupConfig {
		t.Error("WithDisableBackup must clear isBackupConfig")
	}
}

func TestOptions_WithOriginalConfig(t *testing.T) {
	o := options{}
	WithOriginalConfig()(&o)
	if !o.originConfig {
		t.Error("WithOriginalConfig must set originConfig")
	}
}

// ---------------------------------------------------------------------------
// pure helpers
// ---------------------------------------------------------------------------

func TestResolveNamespace(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		key       string
		want      string
	}{
		{name: "key overrides", namespace: "a.yaml", key: "b.yaml", want: "b.yaml"},
		{name: "default first of many", namespace: "a.yaml,b.yaml", key: "", want: "a.yaml"},
		{name: "single namespace", namespace: "only", key: "", want: "only"},
		{name: "empty namespace", namespace: "", key: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &apollo{opt: &options{namespace: tt.namespace}}
			if got := e.resolveNamespace(tt.key); got != tt.want {
				t.Errorf("resolveNamespace(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestFormat_Additional(t *testing.T) {
	tests := []struct {
		namespace string
		want      string
	}{
		{"app", json},      // no dot -> json
		{"app.conf", json}, // unknown suffix -> fallback json
		{"app.properties", json},
		{"app.yaml", yaml},
		{"app.yml", yml},
		{"app.json", json},
		{"a.b.c", json},
	}
	for _, tt := range tests {
		if got := format(tt.namespace); got != tt.want {
			t.Errorf("format(%q) = %q, want %q", tt.namespace, got, tt.want)
		}
	}
}

func TestGenKey_Additional(t *testing.T) {
	tests := []struct {
		ns   string
		sub  string
		want string
	}{
		{"app.unknown", "k", "app.unknown.k"}, // unknown suffix keeps full ns
		{"", "k", "k"},
		{"app.yml", "k", "app.k"},
		{"app.json", "k", "app.k"},
		{"app.properties", "k", "app.k"}, // any known format suffix is stripped
	}
	for _, tt := range tests {
		if got := genKey(tt.ns, tt.sub); got != tt.want {
			t.Errorf("genKey(%q, %q) = %q, want %q", tt.ns, tt.sub, got, tt.want)
		}
	}
}

func TestResolve_Nested(t *testing.T) {
	target := map[string]any{}
	resolve("app.name", "hello", target)
	resolve("app.server.port", 8080, target)

	app, ok := target["app"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested map under app, got %#v", target)
	}
	if app["name"] != "hello" {
		t.Errorf("app.name = %v, want hello", app["name"])
	}
	server, ok := app["server"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested map under app.server, got %#v", app["server"])
	}
	if server["port"] != 8080 {
		t.Errorf("app.server.port = %v, want 8080", server["port"])
	}
}

func TestResolve_DuplicateKey(t *testing.T) {
	target := map[string]any{}
	// "a" is a scalar; re-expanding "a.b" hits the duplicate-key fallback
	// (logs and keeps the existing scalar).
	resolve("a", "scalar", target)
	resolve("a.b", "second", target)
	if target["a"] != "scalar" {
		t.Errorf("duplicate key handling: target = %#v, want a=scalar preserved", target)
	}
}

func TestParsers(t *testing.T) {
	jr, err := (jsonExtParser{}).Parse("raw-json-content")
	if err != nil {
		t.Fatalf("jsonExtParser.Parse error: %v", err)
	}
	if jr["content"] != "raw-json-content" {
		t.Errorf("jsonExtParser.Parse = %#v, want content key", jr)
	}

	yr, err := (yamlExtParser{}).Parse("raw-yaml-content")
	if err != nil {
		t.Fatalf("yamlExtParser.Parse error: %v", err)
	}
	if yr["content"] != "raw-yaml-content" {
		t.Errorf("yamlExtParser.Parse = %#v, want content key", yr)
	}
}

// ---------------------------------------------------------------------------
// getConfig / getOriginConfig / Load
// ---------------------------------------------------------------------------

func newTestApollo(mc *mockClient, opts *options) *apollo {
	return &apollo{client: mc, opt: opts}
}

func TestGetConfig_FlattensNestedKeys(t *testing.T) {
	mc := &mockClient{cache: &mockCache{data: map[string]any{
		"server.port": 9090,
		"server.host": "localhost",
	}}}
	e := newTestApollo(mc, &options{namespace: "app.properties"})

	got, err := e.getConfig("app.properties")
	if err != nil {
		t.Fatalf("getConfig() error = %v", err)
	}
	if mc.namespace != "app.properties" {
		t.Errorf("GetConfigCache namespace = %q, want app.properties", mc.namespace)
	}

	var m map[string]any
	if err := stdjson.Unmarshal(got, &m); err != nil {
		t.Fatalf("getConfig() returned invalid JSON: %v (%s)", err, got)
	}
	// The namespace prefix "app" (from "app.properties") wraps the keys.
	app, ok := m["app"].(map[string]any)
	if !ok {
		t.Fatalf("expected app namespace object, got %#v", m)
	}
	server, ok := app["server"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested server object, got %#v", app)
	}
	if server["port"] != float64(9090) || server["host"] != "localhost" {
		t.Errorf("server = %#v, want port 9090 host localhost", server)
	}
}

func TestGetOriginConfig(t *testing.T) {
	mc := &mockClient{cache: &mockCache{data: map[string]any{
		"content": "name: hello\n",
	}}}
	e := newTestApollo(mc, &options{})

	got, err := e.getOriginConfig("app.yaml")
	if err != nil {
		t.Fatalf("getOriginConfig() error = %v", err)
	}
	if string(got) != "name: hello\n" {
		t.Errorf("getOriginConfig() = %q, want raw content", string(got))
	}
}

func TestGetOriginConfig_Missing(t *testing.T) {
	mc := &mockClient{cache: &mockCache{data: map[string]any{}}}
	e := newTestApollo(mc, &options{})

	if _, err := e.getOriginConfig("app.yaml"); err == nil {
		t.Fatal("getOriginConfig() expected error when content key missing")
	}
}

func TestLoad_OriginConfig(t *testing.T) {
	mc := &mockClient{cache: &mockCache{data: map[string]any{
		"content": `{"name":"hello"}`,
	}}}
	e := newTestApollo(mc, &options{namespace: "app.yaml", originConfig: true})

	got, err := e.Load(context.Background(), "")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if string(got) != `{"name":"hello"}` {
		t.Errorf("Load() = %q, want raw original content", string(got))
	}
}

func TestLoad_MergedConfig(t *testing.T) {
	mc := &mockClient{cache: &mockCache{data: map[string]any{
		"key.only": "value",
	}}}
	e := newTestApollo(mc, &options{namespace: "app.yaml"})

	got, err := e.Load(context.Background(), "")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	var m map[string]any
	if err := stdjson.Unmarshal(got, &m); err != nil {
		t.Fatalf("Load() returned invalid JSON: %v (%s)", err, got)
	}
	app, ok := m["app"].(map[string]any)
	if !ok {
		t.Errorf("Load() = %s, want app namespace object wrapping the keys", got)
	} else if app["key"] == nil {
		t.Errorf("Load() = %s, want nested key object under app", got)
	}
}

func TestLoad_PropertiesNamespace(t *testing.T) {
	mc := &mockClient{cache: &mockCache{data: map[string]any{
		"a.b": 1,
	}}}
	e := newTestApollo(mc, &options{namespace: "app.properties", originConfig: true})

	// originConfig is true but .properties namespaces always use the merged path.
	if _, err := e.Load(context.Background(), ""); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoad_KeySelectsNamespace(t *testing.T) {
	mc := &mockClient{cache: &mockCache{data: map[string]any{
		"content": "x",
	}}}
	e := newTestApollo(mc, &options{namespace: "app.yaml", originConfig: true})

	if _, err := e.Load(context.Background(), "other.yml"); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if mc.namespace != "other.yml" {
		t.Errorf("GetConfigCache namespace = %q, want other.yml", mc.namespace)
	}
}

// ---------------------------------------------------------------------------
// WatchValue
// ---------------------------------------------------------------------------

func TestWatchValue_DeliversChange(t *testing.T) {
	mc := &mockClient{cache: &mockCache{data: map[string]any{}}}
	e := newTestApollo(mc, &options{namespace: "app.yaml"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := e.WatchValue(ctx, "")
	if err != nil {
		t.Fatalf("WatchValue() error = %v", err)
	}
	if len(mc.added) != 1 {
		t.Fatalf("AddChangeListener calls = %d, want 1", len(mc.added))
	}
	listener, ok := mc.added[0].(*valueChangeListener)
	if !ok {
		t.Fatalf("registered listener type = %T, want *valueChangeListener", mc.added[0])
	}
	if listener.namespace != "app.yaml" {
		t.Errorf("listener namespace = %q, want app.yaml", listener.namespace)
	}

	ev := &storage.ChangeEvent{Changes: map[string]*storage.ConfigChange{
		"content": {OldValue: "old", NewValue: `{"name":"new"}`, ChangeType: storage.MODIFIED},
	}}
	ev.Namespace = "app.yaml"
	listener.OnChange(ev)

	select {
	case got := <-ch:
		if string(got) != `{"name":"new"}` {
			t.Errorf("watch got %s, want raw content", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("change not delivered within 2s")
	}

	// Change events for other namespaces must be filtered out.
	ev2 := &storage.ChangeEvent{Changes: map[string]*storage.ConfigChange{
		"content": {NewValue: `{"x":1}`, ChangeType: storage.MODIFIED},
	}}
	ev2.Namespace = "other.yaml"
	listener.OnChange(ev2)
	select {
	case got := <-ch:
		t.Errorf("foreign namespace change unexpectedly delivered: %s", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestWatchValue_CancelClosesAndRemoves(t *testing.T) {
	mc := &mockClient{cache: &mockCache{data: map[string]any{}}}
	e := newTestApollo(mc, &options{namespace: "app.yaml"})

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := e.WatchValue(ctx, "")
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

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(mc.removed) > 0 {
			if mc.removed[0] != mc.added[0] {
				t.Error("RemoveChangeListener received a different listener than the one added")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("RemoveChangeListener was not called after ctx cancel")
}

func TestValueChangeListener_OnNewestChange_Noop(t *testing.T) {
	c := &valueChangeListener{}
	// Must not panic.
	c.OnNewestChange(&storage.FullChangeEvent{})
}

// ---------------------------------------------------------------------------
// config assembly sanity (no dialing: NewSource would dial, so construct directly)
// ---------------------------------------------------------------------------

func TestAppConfigAssembly(t *testing.T) {
	// Mirrors the AppConfig assembly performed by NewSource so the mapping
	// from options stays pinned, without starting the agollo components.
	op := options{}
	WithAppID("demo")(&op)
	WithCluster("dev")(&op)
	WithNamespace("application")(&op)
	WithEndpoint("http://127.0.0.1:8080")(&op)
	WithSecret("k")(&op)

	cfg := &apolloconfig.AppConfig{
		AppID:            op.appid,
		Cluster:          op.cluster,
		NamespaceName:    op.namespace,
		IP:               op.endpoint,
		IsBackupConfig:   op.isBackupConfig,
		Secret:           op.secret,
		BackupConfigPath: op.backupPath,
	}
	if cfg.AppID != "demo" || cfg.Cluster != "dev" || cfg.NamespaceName != "application" ||
		cfg.IP != "http://127.0.0.1:8080" || cfg.Secret != "k" || cfg.IsBackupConfig {
		t.Errorf("AppConfig assembly mismatch: %+v", cfg)
	}
}
