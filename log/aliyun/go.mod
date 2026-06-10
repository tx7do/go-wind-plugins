module github.com/tx7do/go-wind-plugins/log/aliyun

go 1.26.3

require (
	github.com/aliyun/aliyun-log-go-sdk v0.1.100
	github.com/tx7do/go-wind v0.0.1
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/go-kit/kit v0.13.0 // indirect
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-logfmt/logfmt v0.6.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/klauspost/compress v1.18.5 // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/pierrec/lz4/v4 v4.1.26 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/net v0.53.0 // indirect
	gopkg.in/ini.v1 v1.67.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
)

// Redirect the old monolithic genproto to a version where googleapis/rpc
// has been split out, avoiding ambiguous import conflicts.
replace google.golang.org/genproto => google.golang.org/genproto v0.0.0-20260406210006-6f92a3bedf2d

// github.com/lyft/protoc-gen-validate has moved to envoyproxy/protoc-gen-validate.
// The old repository is gone, so we redirect all transitive references.
replace github.com/lyft/protoc-gen-validate => github.com/envoyproxy/protoc-gen-validate v0.10.1
