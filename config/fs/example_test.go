package fs_test

import (
	"context"
	"embed"

	"github.com/tx7do/go-wind-plugins/config/fs"
)

//go:embed testdata/config.yaml
var configFS embed.FS

// ExampleNew constructs a config source backed by an embedded file system. In
// a real application the FS is the application's own config directory baked
// into the binary at build time, so deployments need no external configuration
// service; the one embedded here stands in for it. Load reads the file at the
// configured default path from the embedded FS.
func ExampleNew() {
	src, err := fs.New(
		fs.WithFS(configFS),
		fs.WithPath("testdata/config.yaml"),
	)
	if err != nil {
		return
	}

	raw, err := src.Load(context.Background(), "")
	if err != nil {
		return
	}
	_ = raw
}
