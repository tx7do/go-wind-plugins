module github.com/tx7do/go-wind-plugins/log/aliyun

go 1.26.3

require (
	github.com/aliyun/aliyun-log-go-sdk v0.1.117
	github.com/tx7do/go-wind v0.0.0-20260609092115-0a5df91d8c74
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dennwc/varint v1.0.0 // indirect
	github.com/go-kit/kit v0.13.0 // indirect
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-logfmt/logfmt v0.6.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/grafana/regexp v0.0.0-20250905093917-f7b3be9d1853 // indirect
	github.com/klauspost/compress v1.18.5 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pierrec/lz4/v4 v4.1.26 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_golang v1.23.2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.67.5 // indirect
	github.com/prometheus/procfs v0.20.1 // indirect
	github.com/prometheus/prometheus v0.311.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	golang.org/x/exp v0.0.0-20260410095643-746e56fc9e2f // indirect
	golang.org/x/net v0.52.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
)

// Exclude the old monolithic genproto to avoid ambiguous import with the
// split module google.golang.org/genproto/googleapis/rpc.
exclude google.golang.org/genproto v0.0.0-20210917145530-b395a37504d4

// github.com/lyft/protoc-gen-validate has moved to envoyproxy/protoc-gen-validate.
// The old repository is gone, so we redirect all transitive references.
replace github.com/lyft/protoc-gen-validate => github.com/envoyproxy/protoc-gen-validate v0.10.1
