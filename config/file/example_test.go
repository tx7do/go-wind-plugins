package file_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/config/file"
)

// ExampleNew constructs a file-backed configuration source. Load performs a
// one-shot read; with watching enabled, WatchValue delivers updated file
// contents on the returned channel so configuration can be reloaded live.
func ExampleNew() {
	src, err := file.New(
		file.WithPath("/etc/myapp/config.json"),
		file.WithWatch(true),
	)
	if err != nil {
		return
	}
	defer src.Close()

	raw, err := src.Load(context.Background(), "")
	if err != nil {
		return
	}
	_ = raw

	ch, err := src.WatchValue(context.Background(), "")
	if err != nil {
		return
	}
	_ = ch
}
