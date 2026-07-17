module github.com/tx7do/go-wind-plugins/examples

go 1.26.3

require (
	github.com/99designs/gqlgen v0.17.89
	github.com/apache/thrift v0.23.0
	github.com/googollee/go-socket.io v1.7.0
	github.com/hibiken/asynq v0.26.0
	github.com/prometheus/client_golang v1.23.2
	github.com/tx7do/go-utils/crypto v0.0.2
	github.com/tx7do/go-wind v0.0.1
	github.com/tx7do/go-wind-plugins/broker v0.0.1
	github.com/tx7do/go-wind-plugins/cache v0.0.1
	github.com/tx7do/go-wind-plugins/cache/local v0.0.1
	github.com/tx7do/go-wind-plugins/circuitbreaker v0.0.1
	github.com/tx7do/go-wind-plugins/circuitbreaker/sres v0.0.0-00010101000000-000000000000
	github.com/tx7do/go-wind-plugins/config/file v0.0.1
	github.com/tx7do/go-wind-plugins/encoding/json v0.0.1
	github.com/tx7do/go-wind-plugins/encoding/xml v0.0.1
	github.com/tx7do/go-wind-plugins/health v0.0.0-00010101000000-000000000000
	github.com/tx7do/go-wind-plugins/metrics v0.0.1
	github.com/tx7do/go-wind-plugins/metrics/prometheus v0.0.0-00010101000000-000000000000
	github.com/tx7do/go-wind-plugins/ratelimit v0.0.1
	github.com/tx7do/go-wind-plugins/ratelimit/tokenbucket v0.0.0-00010101000000-000000000000
	github.com/tx7do/go-wind-plugins/registry/etcd v0.0.1
	github.com/tx7do/go-wind-plugins/retry v0.0.0-00010101000000-000000000000
	github.com/tx7do/go-wind-plugins/testing v0.0.1
	github.com/tx7do/go-wind-plugins/tracer/otlp v0.0.1
	github.com/tx7do/go-wind-plugins/transport/asynq v0.0.1
	github.com/tx7do/go-wind-plugins/transport/cron v0.0.1
	github.com/tx7do/go-wind-plugins/transport/graphql v0.0.1
	github.com/tx7do/go-wind-plugins/transport/grpc/middleware/logging v0.0.1
	github.com/tx7do/go-wind-plugins/transport/grpc/middleware/recovery v0.0.1
	github.com/tx7do/go-wind-plugins/transport/grpc/server v0.0.1
	github.com/tx7do/go-wind-plugins/transport/http v0.0.1
	github.com/tx7do/go-wind-plugins/transport/http/driver/std v0.0.1
	github.com/tx7do/go-wind-plugins/transport/http/middleware/codec v0.0.1
	github.com/tx7do/go-wind-plugins/transport/http/middleware/crypto v0.0.1
	github.com/tx7do/go-wind-plugins/transport/http/middleware/logging v0.0.1
	github.com/tx7do/go-wind-plugins/transport/http/middleware/recovery v0.0.1
	github.com/tx7do/go-wind-plugins/transport/http/middleware/requestid v0.0.1
	github.com/tx7do/go-wind-plugins/transport/http/middleware/tracing v0.0.1
		github.com/tx7do/go-wind-plugins/transport/http/swagger v0.0.1
		github.com/tx7do/go-wind-plugins/transport/http/redoc v0.0.1
	github.com/tx7do/go-wind-plugins/transport/kafka v0.0.1
	github.com/tx7do/go-wind-plugins/transport/socketio v0.0.1
	github.com/tx7do/go-wind-plugins/transport/sse v0.0.1
	github.com/tx7do/go-wind-plugins/transport/tcp v0.0.1
	github.com/tx7do/go-wind-plugins/transport/thrift v0.0.1
	github.com/tx7do/go-wind-plugins/transport/websocket v0.0.1
	go.etcd.io/etcd/client/v3 v3.6.10
	go.opentelemetry.io/otel v1.43.0
	go.opentelemetry.io/otel/trace v1.43.0
	google.golang.org/grpc v1.80.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/agnivade/levenshtein v1.2.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bwmarrin/snowflake v0.3.0 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/coocood/freecache v1.2.7 // indirect
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd/v22 v22.7.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/gofrs/uuid v4.4.0+incompatible // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/gomodule/redigo v1.9.3 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/handlers v1.5.2 // indirect
	github.com/gorilla/mux v1.8.1 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.29.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/klauspost/compress v1.18.5 // indirect
	github.com/lithammer/shortuuid/v4 v4.2.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pierrec/lz4/v4 v4.1.26 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.67.5 // indirect
	github.com/prometheus/procfs v0.20.1 // indirect
	github.com/redis/go-redis/v9 v9.18.0 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/rs/xid v1.6.0 // indirect
	github.com/segmentio/kafka-go v0.4.51 // indirect
	github.com/segmentio/ksuid v1.0.4 // indirect
	github.com/sony/sonyflake v1.3.0 // indirect
	github.com/sosodev/duration v1.4.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/tjfoc/gmsm v1.4.1 // indirect
	github.com/tx7do/go-utils v1.1.40 // indirect
	github.com/tx7do/go-utils/id v0.0.6 // indirect
	github.com/tx7do/go-wind-plugins/broker/kafka v0.0.1 // indirect
	github.com/tx7do/go-wind-plugins/config v0.0.1 // indirect
	github.com/tx7do/go-wind-plugins/encoding v0.0.1 // indirect
	github.com/tx7do/go-wind-plugins/encoding/proto v0.0.1 // indirect
	github.com/tx7do/go-wind-plugins/registry v0.0.1 // indirect
	github.com/tx7do/go-wind-plugins/security/crypto v0.0.1 // indirect
	github.com/tx7do/go-wind-plugins/transport v0.0.1 // indirect
	github.com/vektah/gqlparser/v2 v2.5.32 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.2.0 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	go.etcd.io/etcd/api/v3 v3.6.10 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.6.10 // indirect
	go.mongodb.org/mongo-driver/v2 v2.6.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.43.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.43.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.43.0 // indirect
	go.opentelemetry.io/otel/metric v1.43.0 // indirect
	go.opentelemetry.io/otel/sdk v1.43.0 // indirect
	go.opentelemetry.io/proto/otlp v1.10.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260427160629-7cedc36a6bc4 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260427160629-7cedc36a6bc4 // indirect
)

replace (
	github.com/tx7do/go-wind-plugins/broker => ../broker
	github.com/tx7do/go-wind-plugins/broker/kafka => ../broker/kafka
	github.com/tx7do/go-wind-plugins/cache => ../cache
	github.com/tx7do/go-wind-plugins/cache/local => ../cache/local
	github.com/tx7do/go-wind-plugins/circuitbreaker => ../circuitbreaker
	github.com/tx7do/go-wind-plugins/circuitbreaker/sres => ../circuitbreaker/sres
	github.com/tx7do/go-wind-plugins/config => ../config
	github.com/tx7do/go-wind-plugins/config/file => ../config/file
	github.com/tx7do/go-wind-plugins/encoding => ../encoding
	github.com/tx7do/go-wind-plugins/encoding/json => ../encoding/json
	github.com/tx7do/go-wind-plugins/encoding/xml => ../encoding/xml
	github.com/tx7do/go-wind-plugins/health => ../health
	github.com/tx7do/go-wind-plugins/metrics => ../metrics
	github.com/tx7do/go-wind-plugins/metrics/prometheus => ../metrics/prometheus
	github.com/tx7do/go-wind-plugins/ratelimit => ../ratelimit
	github.com/tx7do/go-wind-plugins/ratelimit/tokenbucket => ../ratelimit/tokenbucket
	github.com/tx7do/go-wind-plugins/registry => ../registry
	github.com/tx7do/go-wind-plugins/registry/etcd => ../registry/etcd
	github.com/tx7do/go-wind-plugins/retry => ../retry
	github.com/tx7do/go-wind-plugins/security/crypto => ../security/crypto
	github.com/tx7do/go-wind-plugins/testing => ../testing
	github.com/tx7do/go-wind-plugins/tracer/otlp => ../tracer/otlp
	github.com/tx7do/go-wind-plugins/transport => ../transport
	github.com/tx7do/go-wind-plugins/transport/asynq => ../transport/asynq
	github.com/tx7do/go-wind-plugins/transport/cron => ../transport/cron
	github.com/tx7do/go-wind-plugins/transport/graphql => ../transport/graphql
	github.com/tx7do/go-wind-plugins/transport/grpc/middleware/logging => ../transport/grpc/middleware/logging
	github.com/tx7do/go-wind-plugins/transport/grpc/middleware/recovery => ../transport/grpc/middleware/recovery
	github.com/tx7do/go-wind-plugins/transport/grpc/server => ../transport/grpc/server
	github.com/tx7do/go-wind-plugins/transport/http => ../transport/http
	github.com/tx7do/go-wind-plugins/transport/http/driver/std => ../transport/http/driver/std
	github.com/tx7do/go-wind-plugins/transport/http/middleware/codec => ../transport/http/middleware/codec
	github.com/tx7do/go-wind-plugins/transport/http/middleware/crypto => ../transport/http/middleware/crypto
	github.com/tx7do/go-wind-plugins/transport/http/middleware/logging => ../transport/http/middleware/logging
	github.com/tx7do/go-wind-plugins/transport/http/middleware/recovery => ../transport/http/middleware/recovery
	github.com/tx7do/go-wind-plugins/transport/http/middleware/requestid => ../transport/http/middleware/requestid
	github.com/tx7do/go-wind-plugins/transport/http/middleware/tracing => ../transport/http/middleware/tracing
		github.com/tx7do/go-wind-plugins/transport/http/swagger => ../transport/http/swagger
		github.com/tx7do/go-wind-plugins/transport/http/redoc => ../transport/http/redoc
	github.com/tx7do/go-wind-plugins/transport/kafka => ../transport/kafka
	github.com/tx7do/go-wind-plugins/transport/socketio => ../transport/socketio
	github.com/tx7do/go-wind-plugins/transport/sse => ../transport/sse
	github.com/tx7do/go-wind-plugins/transport/tcp => ../transport/tcp
	github.com/tx7do/go-wind-plugins/transport/thrift => ../transport/thrift
	github.com/tx7do/go-wind-plugins/transport/websocket => ../transport/websocket
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20260406210006-6f92a3bedf2d
)
