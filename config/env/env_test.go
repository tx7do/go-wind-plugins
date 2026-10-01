package env

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const testEnvKey = "ENV_CONFIG_TEST_KEY"

// ---------------------------------------------------------------------------
// Constructor and options
// ---------------------------------------------------------------------------

func TestNew_DefaultOptions(t *testing.T) {
	s, err := New()
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	if s == nil {
		t.Fatal("New() returned nil source")
	}
	if s.options.prefix != "" {
		t.Errorf("default prefix = %q, want empty", s.options.prefix)
	}
	if s.options.key != "" {
		t.Errorf("default key = %q, want empty", s.options.key)
	}
}

func TestWithOptions(t *testing.T) {
	tests := []struct {
		name       string
		opts       []Option
		wantPrefix string
		wantKey    string
	}{
		{
			name:       "no options",
			opts:       nil,
			wantPrefix: "",
			wantKey:    "",
		},
		{
			name:       "prefix only",
			opts:       []Option{WithPrefix("APP_")},
			wantPrefix: "APP_",
			wantKey:    "",
		},
		{
			name:       "key only",
			opts:       []Option{WithKey("DATABASE_URL")},
			wantPrefix: "",
			wantKey:    "DATABASE_URL",
		},
		{
			name:       "prefix and key",
			opts:       []Option{WithPrefix("APP_"), WithKey("DATABASE_URL")},
			wantPrefix: "APP_",
			wantKey:    "DATABASE_URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := New(tt.opts...)
			if err != nil {
				t.Fatalf("New() returned error: %v", err)
			}
			if s.options.prefix != tt.wantPrefix {
				t.Errorf("prefix = %q, want %q", s.options.prefix, tt.wantPrefix)
			}
			if s.options.key != tt.wantKey {
				t.Errorf("key = %q, want %q", s.options.key, tt.wantKey)
			}
		})
	}
}

// TestOptionFunctions verifies each Option mutates the options struct as
// expected when applied directly.
func TestOptionFunctions(t *testing.T) {
	o := &options{}
	WithPrefix("P_")(o)
	if o.prefix != "P_" {
		t.Errorf("WithPrefix set prefix = %q, want %q", o.prefix, "P_")
	}

	WithKey("KEY")(o)
	if o.key != "KEY" {
		t.Errorf("WithKey set key = %q, want %q", o.key, "KEY")
	}

	// Applying an option twice keeps the last value.
	WithPrefix("Q_")(o)
	if o.prefix != "Q_" {
		t.Errorf("second WithPrefix set prefix = %q, want %q", o.prefix, "Q_")
	}
}

// ---------------------------------------------------------------------------
// resolveKey
// ---------------------------------------------------------------------------

func TestResolveKey(t *testing.T) {
	tests := []struct {
		name       string
		prefix     string
		defaultKey string
		key        string
		want       string
	}{
		{"explicit key, no prefix", "", "DEFAULT", "EXPLICIT", "EXPLICIT"},
		{"explicit key with prefix", "APP_", "DEFAULT", "EXPLICIT", "APP_EXPLICIT"},
		{"default key, no prefix", "", "DEFAULT", "", "DEFAULT"},
		{"default key with prefix", "APP_", "DEFAULT", "", "APP_DEFAULT"},
		{"no key at all", "", "", "", ""},
		{"prefix but no key", "APP_", "", "", ""},
		{"empty prefix ignored", "", "DEFAULT", "", "DEFAULT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &source{options: &options{prefix: tt.prefix, key: tt.defaultKey}}
			if got := s.resolveKey(tt.key); got != tt.want {
				t.Errorf("resolveKey(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Load
// ---------------------------------------------------------------------------

func TestLoad_ExplicitKey(t *testing.T) {
	t.Setenv(testEnvKey, "env-value")

	s, err := New()
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	data, err := s.Load(context.Background(), testEnvKey)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if string(data) != "env-value" {
		t.Errorf("Load = %q, want %q", string(data), "env-value")
	}
}

func TestLoad_DefaultKey(t *testing.T) {
	t.Setenv(testEnvKey, "default-key-value")

	s, err := New(WithKey(testEnvKey))
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	data, err := s.Load(context.Background(), "")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if string(data) != "default-key-value" {
		t.Errorf("Load = %q, want %q", string(data), "default-key-value")
	}
}

func TestLoad_PrefixApplied(t *testing.T) {
	prefixed := "APP_" + testEnvKey
	t.Setenv(prefixed, "prefixed-value")
	t.Setenv(testEnvKey, "unprefixed-value")

	s, err := New(WithPrefix("APP_"))
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	// An explicit key gets the prefix prepended.
	data, err := s.Load(context.Background(), testEnvKey)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if string(data) != "prefixed-value" {
		t.Errorf("Load with prefix = %q, want %q", string(data), "prefixed-value")
	}

	// The unprefixed variable must not be returned.
	if string(data) == "unprefixed-value" {
		t.Error("Load should read the prefixed variable, not the unprefixed one")
	}
}

func TestLoad_PrefixAppliedToDefaultKey(t *testing.T) {
	prefixed := "APP_" + testEnvKey
	t.Setenv(prefixed, "prefixed-default")

	s, err := New(WithPrefix("APP_"), WithKey(testEnvKey))
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	data, err := s.Load(context.Background(), "")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if string(data) != "prefixed-default" {
		t.Errorf("Load = %q, want %q", string(data), "prefixed-default")
	}
}

func TestLoad_UnsetVariable(t *testing.T) {
	// Make sure the variable is not set in the test environment.
	t.Setenv("ENV_CONFIG_TEST_UNSET", "sentinel")
	s, err := New()
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	data, err := s.Load(context.Background(), "ENV_CONFIG_TEST_DEFINITELY_NOT_SET_12345")
	if err != nil {
		t.Fatalf("Load returned error for unset variable: %v", err)
	}
	if data != nil {
		t.Errorf("Load = %q, want nil for unset variable", string(data))
	}
}

func TestLoad_NoKeySpecified(t *testing.T) {
	s, err := New()
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	data, err := s.Load(context.Background(), "")
	if err == nil {
		t.Fatal("Load should fail when no key is specified")
	}
	if data != nil {
		t.Errorf("Load = %q, want nil on error", string(data))
	}
	if !strings.Contains(err.Error(), "no key specified") {
		t.Errorf("Load error = %q, want it to mention %q", err.Error(), "no key specified")
	}
}

func TestLoad_EmptyValue(t *testing.T) {
	// A variable that is set to the empty string is found (LookupEnv returns
	// ok=true), so Load must return empty bytes rather than nil.
	t.Setenv("ENV_CONFIG_TEST_EMPTY", "")

	s, err := New()
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	data, err := s.Load(context.Background(), "ENV_CONFIG_TEST_EMPTY")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if data == nil {
		t.Error("Load should return non-nil empty bytes for a set-but-empty variable")
	}
	if len(data) != 0 {
		t.Errorf("Load = %q, want empty", string(data))
	}
}

// TestLoad_ContextIgnored documents that Load does not use the context.
func TestLoad_ContextIgnored(t *testing.T) {
	t.Setenv(testEnvKey, "value")

	s, err := New()
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled context must not affect the lookup

	data, err := s.Load(ctx, testEnvKey)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if string(data) != "value" {
		t.Errorf("Load = %q, want %q", string(data), "value")
	}
}

// TestLookupEnv verifies the thin os.LookupEnv wrapper directly.
func TestLookupEnv(t *testing.T) {
	t.Setenv(testEnvKey, "wrapper-value")

	val, ok := lookupEnv(testEnvKey)
	if !ok || val != "wrapper-value" {
		t.Errorf("lookupEnv = (%q, %v), want (%q, true)", val, ok, "wrapper-value")
	}

	if _, ok := lookupEnv("ENV_CONFIG_TEST_DEFINITELY_NOT_SET_12345"); ok {
		t.Error("lookupEnv should report false for an unset variable")
	}
}

// TestReaderInterfaceCompliance pins the source to the base config Reader
// interface.
func TestReaderInterfaceCompliance(t *testing.T) {
	s, err := New()
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	var _ interface {
		Load(ctx context.Context, key string) ([]byte, error)
	} = s
}

// TestLoad_ErrorIsPlainError makes sure a missing key surfaces a regular
// error (not typed), which callers can inspect with errors.New-style checks.
func TestLoad_ErrorIsPlainError(t *testing.T) {
	s, err := New()
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	_, err = s.Load(context.Background(), "")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, err) {
		t.Error("error should be comparable to itself")
	}
}
