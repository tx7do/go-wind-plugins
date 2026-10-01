package option

import (
	"testing"
	"time"

	"github.com/tx7do/go-wind-plugins/broker"
)

// newTestOptions returns fresh broker options for a test.
func newTestOptions() *broker.Options {
	opts := broker.NewOptions()
	return &opts
}

// commonOptsFrom applies the given broker options and extracts the
// CommonOptions stored in the resulting context.
func commonOptsFrom(t *testing.T, opts ...broker.Option) *CommonOptions {
	t.Helper()

	o := newTestOptions()
	for _, opt := range opts {
		opt(o)
	}

	v, ok := o.Context.Value(OptionsKey).(*CommonOptions)
	if !ok || v == nil {
		t.Fatal("expected CommonOptions stored under OptionsKey in broker options context")
	}
	return v
}

// ---------------------------------------------------------------------------
// Defaults
// ---------------------------------------------------------------------------

func TestDefaultConstantValues(t *testing.T) {
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"DefaultMaxIdle", DefaultMaxIdle, 256},
		{"DefaultMaxActive", DefaultMaxActive, 0},
		{"DefaultIdleTimeout", DefaultIdleTimeout, time.Duration(0)},
		{"DefaultConnectTimeout", DefaultConnectTimeout, 30 * time.Second},
		{"DefaultReadTimeout", DefaultReadTimeout, 30 * time.Second},
		{"DefaultWriteTimeout", DefaultWriteTimeout, 30 * time.Second},
		{"DefaultHealthCheckPeriod", DefaultHealthCheckPeriod, time.Minute},
		{"DefaultStreamGroup", DefaultStreamGroup, "wind-group"},
		{"DefaultStreamConsumer", DefaultStreamConsumer, "wind-consumer"},
		{"DefaultStreamBlockTime", DefaultStreamBlockTime, 5 * time.Second},
		{"DefaultStreamCount", DefaultStreamCount, 10},
		{"DefaultStreamMaxLen", DefaultStreamMaxLen, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestDriverTypeValues(t *testing.T) {
	if DriverTypePubSub != "pubsub" {
		t.Errorf("DriverTypePubSub = %q, want %q", DriverTypePubSub, "pubsub")
	}
	if DriverTypeStream != "stream" {
		t.Errorf("DriverTypeStream = %q, want %q", DriverTypeStream, "stream")
	}
}

func TestWithDefaultOptions(t *testing.T) {
	got := commonOptsFrom(t, WithDefaultOptions())

	want := &CommonOptions{
		MaxIdle:        DefaultMaxIdle,
		MaxActive:      DefaultMaxActive,
		IdleTimeout:    DefaultIdleTimeout,
		ConnectTimeout: DefaultConnectTimeout,
		ReadTimeout:    DefaultReadTimeout,
		WriteTimeout:   DefaultWriteTimeout,
	}
	if *got != *want {
		t.Errorf("WithDefaultOptions() = %+v, want %+v", got, want)
	}
}

// ---------------------------------------------------------------------------
// With* connection pool options
// ---------------------------------------------------------------------------

func TestWithPoolOptions_MutateOnlyOwnField(t *testing.T) {
	tests := []struct {
		name   string
		apply  broker.Option
		verify func(*CommonOptions) bool
	}{
		{
			name:  "WithConnectTimeout",
			apply: WithConnectTimeout(11 * time.Second),
			verify: func(c *CommonOptions) bool {
				return c.ConnectTimeout == 11*time.Second
			},
		},
		{
			name:  "WithReadTimeout",
			apply: WithReadTimeout(12 * time.Second),
			verify: func(c *CommonOptions) bool {
				return c.ReadTimeout == 12*time.Second
			},
		},
		{
			name:  "WithWriteTimeout",
			apply: WithWriteTimeout(13 * time.Second),
			verify: func(c *CommonOptions) bool {
				return c.WriteTimeout == 13*time.Second
			},
		},
		{
			name:  "WithIdleTimeout",
			apply: WithIdleTimeout(14 * time.Second),
			verify: func(c *CommonOptions) bool {
				return c.IdleTimeout == 14*time.Second
			},
		},
		{
			name:  "WithMaxIdle",
			apply: WithMaxIdle(7),
			verify: func(c *CommonOptions) bool {
				return c.MaxIdle == 7
			},
		},
		{
			name:  "WithMaxActive",
			apply: WithMaxActive(9),
			verify: func(c *CommonOptions) bool {
				return c.MaxActive == 9
			},
		},
		{
			name:  "WithPassword",
			apply: WithPassword("secret"),
			verify: func(c *CommonOptions) bool {
				return c.Password == "secret"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := commonOptsFrom(t, tt.apply)
			if !tt.verify(got) {
				t.Errorf("%s produced %+v, own field not applied", tt.name, got)
			}

			// All other fields must keep their defaults (a lone With* must not
			// clobber the remaining pool defaults with zero values).
			if got.MaxIdle != DefaultMaxIdle && tt.name != "WithMaxIdle" {
				t.Errorf("MaxIdle = %d, want default %d", got.MaxIdle, DefaultMaxIdle)
			}
			if got.ConnectTimeout != DefaultConnectTimeout && tt.name != "WithConnectTimeout" {
				t.Errorf("ConnectTimeout = %v, want default %v", got.ConnectTimeout, DefaultConnectTimeout)
			}
			if got.ReadTimeout != DefaultReadTimeout && tt.name != "WithReadTimeout" {
				t.Errorf("ReadTimeout = %v, want default %v", got.ReadTimeout, DefaultReadTimeout)
			}
			if got.WriteTimeout != DefaultWriteTimeout && tt.name != "WithWriteTimeout" {
				t.Errorf("WriteTimeout = %v, want default %v", got.WriteTimeout, DefaultWriteTimeout)
			}
		})
	}
}

func TestWithOptions_AccumulateOnSameStruct(t *testing.T) {
	// Multiple With* options applied to the same Options must share one
	// CommonOptions instance, so every mutation is visible.
	got := commonOptsFrom(t,
		WithConnectTimeout(time.Second),
		WithReadTimeout(2*time.Second),
		WithMaxIdle(3),
		WithMaxActive(4),
		WithPassword("pw"),
	)

	if got.ConnectTimeout != time.Second {
		t.Errorf("ConnectTimeout = %v, want %v", got.ConnectTimeout, time.Second)
	}
	if got.ReadTimeout != 2*time.Second {
		t.Errorf("ReadTimeout = %v, want %v", got.ReadTimeout, 2*time.Second)
	}
	if got.MaxIdle != 3 {
		t.Errorf("MaxIdle = %d, want 3", got.MaxIdle)
	}
	if got.MaxActive != 4 {
		t.Errorf("MaxActive = %d, want 4", got.MaxActive)
	}
	if got.Password != "pw" {
		t.Errorf("Password = %q, want %q", got.Password, "pw")
	}
}

func TestWithDefaultOptions_ResetsPreviousValues(t *testing.T) {
	o := newTestOptions()
	WithMaxIdle(1)(o)
	WithPassword("x")(o)
	WithDefaultOptions()(o)

	v, ok := o.Context.Value(OptionsKey).(*CommonOptions)
	if !ok || v != nil && v.MaxIdle != DefaultMaxIdle {
		if v == nil {
			t.Fatal("expected CommonOptions after WithDefaultOptions")
		}
		t.Errorf("MaxIdle = %d, want default %d after reset", v.MaxIdle, DefaultMaxIdle)
	}
	if v.Password != "" {
		t.Errorf("Password = %q, want empty after reset", v.Password)
	}
}

// ---------------------------------------------------------------------------
// Stream options
// ---------------------------------------------------------------------------

func TestStreamSubscribeOptions(t *testing.T) {
	tests := []struct {
		name  string
		apply broker.SubscribeOption
		key   any
		want  any
	}{
		{"WithStreamGroup", WithStreamGroup("g1"), StreamGroupKey{}, "g1"},
		{"WithStreamConsumer", WithStreamConsumer("c1"), StreamConsumerKey{}, "c1"},
		{"WithStreamBlockTime", WithStreamBlockTime(3 * time.Second), StreamBlockTimeKey{}, 3 * time.Second},
		{"WithStreamCount", WithStreamCount(42), StreamCountKey{}, 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := broker.NewSubscribeOptions(tt.apply)

			got := o.Context.Value(tt.key)
			if got != tt.want {
				t.Errorf("%s: context value = %v (%T), want %v", tt.name, got, got, tt.want)
			}
		})
	}
}

func TestWithStreamMaxLen(t *testing.T) {
	o := broker.NewPublishOptions(WithStreamMaxLen(1234))

	got, ok := o.Context.Value(StreamMaxLenKey{}).(int64)
	if !ok {
		t.Fatalf("StreamMaxLenKey value = %T, want int64", o.Context.Value(StreamMaxLenKey{}))
	}
	if got != 1234 {
		t.Errorf("WithStreamMaxLen: got %d, want 1234", got)
	}
}
