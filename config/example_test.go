package config_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/config"
)

// stubReader is a minimal config.Reader standing in for a real provider such
// as a file source or a remote configuration center.
type stubReader struct {
	data []byte
}

func (s *stubReader) Load(_ context.Context, _ string) ([]byte, error) {
	return s.data, nil
}

// ExampleNewFallbackReader composes several configuration sources into a
// cascading fallback. Load tries the sub-sources in priority order — the first
// one that resolves the key wins — so a higher-priority source shadows the
// others while they still serve the keys it lacks. The readers below stand in
// for real providers, which come from their own modules and are imported next
// to this one in an application; Close releases every sub-source that holds
// resources when the application shuts down.
func ExampleNewFallbackReader() {
	primary := &stubReader{data: []byte("higher-priority source")}
	secondary := &stubReader{data: []byte("lower-priority source")}

	fallback, err := config.NewFallbackReader(primary, secondary)
	if err != nil {
		return
	}
	defer fallback.Close()

	raw, err := fallback.Load(context.Background(), "myapp/config")
	if err != nil {
		return
	}
	_ = raw
}
