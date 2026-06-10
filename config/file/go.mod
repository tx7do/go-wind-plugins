module github.com/tx7do/go-wind-plugins/config/file

go 1.26.3

require (
	github.com/fsnotify/fsnotify v1.10.1
	github.com/tx7do/go-wind v0.0.1
	github.com/tx7do/go-wind-plugins/config v0.0.0-00010101000000-000000000000
)

require golang.org/x/sys v0.43.0 // indirect

replace github.com/tx7do/go-wind-plugins/config => ../
