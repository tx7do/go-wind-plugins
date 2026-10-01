package redis

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/tx7do/go-wind-plugins/cache"
)

// ---------------------------------------------------------------------------
// Mock UniversalClient — only the methods the adapter calls are implemented;
// the embedded nil interface panics on anything else, which is fine because
// the tests never call those.
// ---------------------------------------------------------------------------

type mockClient struct {
	goredis.UniversalClient

	getVal    string
	getErr    error
	getKey    string
	getCount  int
	setKey    string
	setVal    []byte
	setTTL    time.Duration
	setErr    error
	setnxOK   bool
	setnxKey  string
	setnxErr  error
	delErr    error
	delKey    string
	existsN   int64
	existsErr error
	existsKey string
	mgetRepl  []any
	mgetErr   error
	mgetKeys  []string
	pipeline  *mockPipeliner
	pipelineN int
}

func (m *mockClient) Get(_ context.Context, key string) *goredis.StringCmd {
	m.getCount++
	m.getKey = key
	cmd := goredis.NewStringCmd(context.Background())
	cmd.SetVal(m.getVal)
	cmd.SetErr(m.getErr)
	return cmd
}

func (m *mockClient) Set(_ context.Context, key string, value any, expiration time.Duration) *goredis.StatusCmd {
	m.setKey = key
	m.setVal, _ = value.([]byte)
	m.setTTL = expiration
	cmd := goredis.NewStatusCmd(context.Background())
	if m.setErr != nil {
		cmd.SetErr(m.setErr)
	} else {
		cmd.SetVal("OK")
	}
	return cmd
}

func (m *mockClient) SetNX(_ context.Context, key string, _ any, _ time.Duration) *goredis.BoolCmd {
	m.setnxKey = key
	cmd := goredis.NewBoolCmd(context.Background())
	if m.setnxErr != nil {
		cmd.SetErr(m.setnxErr)
	} else {
		cmd.SetVal(m.setnxOK)
	}
	return cmd
}

func (m *mockClient) Del(_ context.Context, keys ...string) *goredis.IntCmd {
	m.delKey = keys[0]
	cmd := goredis.NewIntCmd(context.Background())
	if m.delErr != nil {
		cmd.SetErr(m.delErr)
	} else {
		cmd.SetVal(1)
	}
	return cmd
}

func (m *mockClient) Exists(_ context.Context, keys ...string) *goredis.IntCmd {
	m.existsKey = keys[0]
	cmd := goredis.NewIntCmd(context.Background())
	if m.existsErr != nil {
		cmd.SetErr(m.existsErr)
	} else {
		cmd.SetVal(m.existsN)
	}
	return cmd
}

func (m *mockClient) MGet(_ context.Context, keys ...string) *goredis.SliceCmd {
	m.mgetKeys = keys
	cmd := goredis.NewSliceCmd(context.Background())
	if m.mgetErr != nil {
		cmd.SetErr(m.mgetErr)
	} else {
		cmd.SetVal(m.mgetRepl)
	}
	return cmd
}

func (m *mockClient) Pipeline() goredis.Pipeliner {
	m.pipelineN++
	if m.pipeline == nil {
		m.pipeline = &mockPipeliner{}
	}
	return m.pipeline
}

type mockPipeliner struct {
	goredis.StatefulCmdable

	sets    []string
	cmds    []goredis.Cmder // commands queued via Set, returned by Exec
	setErr  error
	execErr error
}

func (p *mockPipeliner) Set(_ context.Context, key string, value any, _ time.Duration) *goredis.StatusCmd {
	p.sets = append(p.sets, key)
	cmd := goredis.NewStatusCmd(context.Background())
	if p.setErr != nil {
		cmd.SetErr(p.setErr)
	} else {
		cmd.SetVal("OK")
	}
	p.cmds = append(p.cmds, cmd)
	return cmd
}

func (p *mockPipeliner) BatchProcess(_ context.Context, _ ...goredis.Cmder) error {
	return nil
}

func (p *mockPipeliner) Cmds() []goredis.Cmder { return nil }

func (p *mockPipeliner) Discard() {}

func (p *mockPipeliner) Do(_ context.Context, _ ...any) *goredis.Cmd {
	return goredis.NewCmd(context.Background())
}

func (p *mockPipeliner) Len() int { return len(p.sets) }

func (p *mockPipeliner) Process(_ context.Context, _ goredis.Cmder) error { return nil }

func (p *mockPipeliner) Exec(_ context.Context) ([]goredis.Cmder, error) {
	// Real go-redis returns every queued command plus a general error when
	// any command failed.
	return p.cmds, p.execErr
}

func newTestCache(mc *mockClient, opts ...Option) *Cache {
	return New(mc, opts...)
}

// ---------------------------------------------------------------------------
// Constructor / options
// ---------------------------------------------------------------------------

func TestNew_NoOptions(t *testing.T) {
	mc := &mockClient{}
	c := newTestCache(mc)

	// Without a prefix, keys are passed through unchanged.
	_, _ = c.Get(context.Background(), "plain-key")
	if mc.getKey != "plain-key" {
		t.Errorf("Get key = %q, want %q", mc.getKey, "plain-key")
	}
}

func TestNew_WithKeyPrefix(t *testing.T) {
	mc := &mockClient{}
	c := newTestCache(mc, WithKeyPrefix("myapp:"))

	_, _ = c.Get(context.Background(), "user:1")
	if mc.getKey != "myapp:user:1" {
		t.Errorf("Get key = %q, want %q", mc.getKey, "myapp:user:1")
	}

	_ = c.Set(context.Background(), "user:1", []byte("v"), 0)
	if mc.setKey != "myapp:user:1" {
		t.Errorf("Set key = %q, want %q", mc.setKey, "myapp:user:1")
	}

	_, _ = c.Has(context.Background(), "user:1")
	if mc.existsKey != "myapp:user:1" {
		t.Errorf("Exists key = %q, want %q", mc.existsKey, "myapp:user:1")
	}

	_ = c.Delete(context.Background(), "user:1")
	if mc.delKey != "myapp:user:1" {
		t.Errorf("Del key = %q, want %q", mc.delKey, "myapp:user:1")
	}
}

func TestCache_ImplementsCacheInterface(t *testing.T) {
	var _ cache.Cache = (*Cache)(nil)
}

func TestClose_ReturnsNil(t *testing.T) {
	c := newTestCache(&mockClient{})
	if err := c.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

func TestGet_Hit(t *testing.T) {
	mc := &mockClient{getVal: "stored"}
	c := newTestCache(mc)

	val, err := c.Get(context.Background(), "k")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if string(val) != "stored" {
		t.Errorf("Get() = %q, want %q", val, "stored")
	}
}

func TestGet_NotFound(t *testing.T) {
	mc := &mockClient{getErr: goredis.Nil}
	c := newTestCache(mc)

	val, err := c.Get(context.Background(), "missing")
	if !errors.Is(err, cache.ErrNotFound) {
		t.Errorf("Get() error = %v, want cache.ErrNotFound", err)
	}
	if val != nil {
		t.Errorf("Get() value = %v, want nil", val)
	}
}

func TestGet_OtherErrorPropagates(t *testing.T) {
	boom := errors.New("boom")
	mc := &mockClient{getErr: boom}
	c := newTestCache(mc)

	if _, err := c.Get(context.Background(), "k"); !errors.Is(err, boom) {
		t.Errorf("Get() error = %v, want %v", err, boom)
	}
}

// ---------------------------------------------------------------------------
// Set / SetNX / Delete / Has
// ---------------------------------------------------------------------------

func TestSet(t *testing.T) {
	mc := &mockClient{}
	c := newTestCache(mc)

	ttl := 10 * time.Minute
	if err := c.Set(context.Background(), "k", []byte("v"), ttl); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if mc.setKey != "k" || string(mc.setVal) != "v" || mc.setTTL != ttl {
		t.Errorf("Set args = key %q val %q ttl %v, want k/v/%v", mc.setKey, mc.setVal, mc.setTTL, ttl)
	}
}

func TestSet_ErrorPropagates(t *testing.T) {
	boom := errors.New("boom")
	mc := &mockClient{setErr: boom}
	c := newTestCache(mc)

	if err := c.Set(context.Background(), "k", []byte("v"), 0); !errors.Is(err, boom) {
		t.Errorf("Set() error = %v, want %v", err, boom)
	}
}

func TestSetNX(t *testing.T) {
	tests := []struct {
		name    string
		mockOK  bool
		mockErr error
		wantOK  bool
		wantErr bool
	}{
		{"acquired", true, nil, true, false},
		{"already held", false, nil, false, false},
		{"error", false, errors.New("boom"), false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &mockClient{setnxOK: tt.mockOK, setnxErr: tt.mockErr}
			c := newTestCache(mc)

			ok, err := c.SetNX(context.Background(), "lock", []byte("v"), time.Second)
			if ok != tt.wantOK {
				t.Errorf("SetNX() ok = %v, want %v", ok, tt.wantOK)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("SetNX() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	mc := &mockClient{}
	c := newTestCache(mc)

	if err := c.Delete(context.Background(), "k"); err != nil {
		t.Errorf("Delete() error = %v", err)
	}
	if mc.delKey != "k" {
		t.Errorf("Del key = %q, want k", mc.delKey)
	}

	mc2 := &mockClient{delErr: errors.New("boom")}
	if err := newTestCache(mc2).Delete(context.Background(), "k"); err == nil {
		t.Error("Delete() should propagate the client error")
	}
}

func TestHas(t *testing.T) {
	tests := []struct {
		name    string
		existsN int64
		want    bool
	}{
		{"exists", 1, true},
		{"missing", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &mockClient{existsN: tt.existsN}
			c := newTestCache(mc)

			ok, err := c.Has(context.Background(), "k")
			if err != nil {
				t.Fatalf("Has() error = %v", err)
			}
			if ok != tt.want {
				t.Errorf("Has() = %v, want %v", ok, tt.want)
			}
		})
	}

	// Error propagation.
	mc := &mockClient{existsErr: errors.New("boom")}
	if _, err := newTestCache(mc).Has(context.Background(), "k"); err == nil {
		t.Error("Has() should propagate the client error")
	}
}

// ---------------------------------------------------------------------------
// GetMulti
// ---------------------------------------------------------------------------

func TestGetMulti_Empty(t *testing.T) {
	c := newTestCache(&mockClient{})

	vals, err := c.GetMulti(context.Background(), nil)
	if err != nil {
		t.Errorf("GetMulti(nil) error = %v, want nil", err)
	}
	if vals != nil {
		t.Errorf("GetMulti(nil) = %v, want nil", vals)
	}

	vals, err = c.GetMulti(context.Background(), []string{})
	if err != nil || vals != nil {
		t.Errorf("GetMulti(empty) = %v, %v, want nil, nil", vals, err)
	}
}

func TestGetMulti_MixedResults(t *testing.T) {
	tests := []struct {
		name    string
		replies []any
		want    [][]byte
		wantErr bool
	}{
		{
			name:    "string values",
			replies: []any{"a", "b"},
			want:    [][]byte{[]byte("a"), []byte("b")},
		},
		{
			name:    "byte slice values",
			replies: []any{[]byte("a"), []byte("b")},
			want:    [][]byte{[]byte("a"), []byte("b")},
		},
		{
			name:    "missing entry yields nil and ErrNotFound",
			replies: []any{"a", nil},
			want:    [][]byte{[]byte("a"), nil},
			wantErr: true,
		},
		{
			name:    "unexpected type treated as missing",
			replies: []any{42},
			want:    [][]byte{nil},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &mockClient{mgetRepl: tt.replies}
			c := newTestCache(mc)

			vals, err := c.GetMulti(context.Background(), []string{"k1", "k2", "k3"}[:len(tt.replies)])
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetMulti() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, cache.ErrNotFound) {
				t.Errorf("GetMulti() error = %v, want cache.ErrNotFound", err)
			}
			if !reflect.DeepEqual(vals, tt.want) {
				t.Errorf("GetMulti() = %v, want %v", vals, tt.want)
			}

			// Keys must be passed through with prefix applied.
			if len(mc.mgetKeys) != len(tt.replies) {
				t.Fatalf("MGet keys = %v, want %d keys", mc.mgetKeys, len(tt.replies))
			}
		})
	}
}

func TestGetMulti_PrefixedKeys(t *testing.T) {
	mc := &mockClient{mgetRepl: []any{"a"}}
	c := newTestCache(mc, WithKeyPrefix("p:"))

	_, _ = c.GetMulti(context.Background(), []string{"k1"})
	if len(mc.mgetKeys) != 1 || mc.mgetKeys[0] != "p:k1" {
		t.Errorf("MGet keys = %v, want [p:k1]", mc.mgetKeys)
	}
}

func TestGetMulti_ErrorPropagates(t *testing.T) {
	boom := errors.New("boom")
	mc := &mockClient{mgetErr: boom}
	c := newTestCache(mc)

	if _, err := c.GetMulti(context.Background(), []string{"k"}); !errors.Is(err, boom) {
		t.Errorf("GetMulti() error = %v, want %v", err, boom)
	}
}

// ---------------------------------------------------------------------------
// SetMulti
// ---------------------------------------------------------------------------

func TestSetMulti_Empty(t *testing.T) {
	mc := &mockClient{}
	c := newTestCache(mc)

	if err := c.SetMulti(context.Background(), nil); err != nil {
		t.Errorf("SetMulti(nil) error = %v, want nil", err)
	}
	if err := c.SetMulti(context.Background(), []cache.Item{}); err != nil {
		t.Errorf("SetMulti(empty) error = %v, want nil", err)
	}
	if mc.pipelineN != 0 {
		t.Error("SetMulti with no items must not build a pipeline")
	}
}

func TestSetMulti_PipelinesAllKeys(t *testing.T) {
	mc := &mockClient{}
	c := newTestCache(mc, WithKeyPrefix("p:"))

	items := []cache.Item{
		{Key: "a", Value: []byte("va"), TTL: time.Minute},
		{Key: "b", Value: []byte("vb"), TTL: 0},
	}

	if err := c.SetMulti(context.Background(), items); err != nil {
		t.Fatalf("SetMulti() error = %v", err)
	}

	pipe := mc.pipeline
	if len(pipe.sets) != 2 {
		t.Fatalf("pipelined sets = %v, want 2 entries", pipe.sets)
	}
	if pipe.sets[0] != "p:a" || pipe.sets[1] != "p:b" {
		t.Errorf("pipelined sets = %v, want [p:a p:b]", pipe.sets)
	}
}

func TestSetMulti_ExecErrorReturnsFirstCommandError(t *testing.T) {
	cmdErr := errors.New("command failed")
	mc := &mockClient{}
	mc.pipeline = &mockPipeliner{setErr: cmdErr, execErr: errors.New("exec failed")}
	c := newTestCache(mc)

	err := c.SetMulti(context.Background(), []cache.Item{{Key: "k", Value: []byte("v")}})
	if !errors.Is(err, cmdErr) {
		t.Errorf("SetMulti() error = %v, want the first failed command error %v", err, cmdErr)
	}
}
