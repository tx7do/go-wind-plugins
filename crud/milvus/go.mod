module github.com/tx7do/go-wind-plugins/crud/milvus

go 1.26.3

replace github.com/tx7do/go-wind-plugins/crud/vector => ../vector

replace github.com/tx7do/go-wind-plugins/crud/viewer => ../viewer

require (
	github.com/milvus-io/milvus-sdk-go/v2 v2.4.2
	github.com/stretchr/testify v1.12.1
	github.com/tx7do/go-utils/mapper v0.0.3
	github.com/tx7do/go-wind v0.0.3
	github.com/tx7do/go-wind-plugins/crud/vector v0.0.1
	github.com/tx7do/go-wind-plugins/crud/viewer v0.0.1
	google.golang.org/grpc v1.79.3
)

require (
	github.com/cockroachdb/errors v1.14.0 // indirect
	github.com/cockroachdb/logtags v0.0.0-20230118201751-21c54148d20b // indirect
	github.com/cockroachdb/redact v1.1.5 // indirect
	github.com/getsentry/sentry-go v0.46.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0 // indirect
	github.com/jinzhu/copier v0.4.0 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/milvus-io/milvus-proto/go-api/v2 v2.4.10-0.20240819025435-512e3b98866a // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/rogpeppe/go-internal v1.10.0 // indirect
	github.com/tidwall/gjson v1.14.4 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	google.golang.org/genproto v0.0.0-20230410155749-daa745c078e1 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
)
