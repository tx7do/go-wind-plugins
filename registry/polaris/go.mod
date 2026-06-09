module github.com/tx7do/go-wind-plugins/registry/polaris

go 1.26.3

require (
	github.com/polarismesh/polaris-go v1.7.0
	github.com/tx7do/go-wind v0.0.0-20260609092115-0a5df91d8c74
	github.com/tx7do/go-wind-plugins/registry v0.0.0-00010101000000-000000000000
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/dlclark/regexp2 v1.7.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/uuid v1.4.0 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/natefinch/lumberjack v2.0.0+incompatible // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/polarismesh/specification v1.7.0 // indirect
	github.com/prometheus/client_golang v1.12.2 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.32.1 // indirect
	github.com/prometheus/procfs v0.7.3 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	go.uber.org/zap v1.21.0 // indirect
	golang.org/x/net v0.33.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240102182953-50ed04b92917 // indirect
	google.golang.org/grpc v1.61.1 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

// Exclude the old monolithic genproto to avoid ambiguous import with the
// split module google.golang.org/genproto/googleapis/rpc.
exclude google.golang.org/genproto v0.0.0-20210602131652-f16073e35f0c

replace github.com/tx7do/go-wind-plugins/registry => ../
