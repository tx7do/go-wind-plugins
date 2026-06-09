module github.com/tx7do/go-wind-plugins/registry/zookeeper

go 1.26.3

require (
	github.com/go-zookeeper/zk v1.0.4
	github.com/tx7do/go-wind v0.0.0-20260609092115-0a5df91d8c74
	github.com/tx7do/go-wind-plugins/registry v0.0.0-00010101000000-000000000000
	golang.org/x/sync v0.20.0
)

replace github.com/tx7do/go-wind-plugins/registry => ../
