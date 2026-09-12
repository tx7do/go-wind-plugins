<div align="center">

<img src="docs/brand/vortex-tile.svg" width="120" alt="Go Wind Plugins" />

# Go Wind Plugins

**English** | [中文](./README.md) | [日本語](./README_ja.md)

</div>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=Go" alt="Go Version" />
  <img src="https://img.shields.io/badge/Framework-go--wind-00ADD8?style=flat-square" alt="go-wind" />
  <img src="https://img.shields.io/badge/License-MIT-green?style=flat-square" alt="License" />
  <img src="https://img.shields.io/badge/PRs-Welcome-brightgreen?style=flat-square" alt="PRs Welcome" />
</p>

---

## Overview

**go-wind-plugins** is the official plugin library for the [go-wind](https://github.com/tx7do/go-wind) microservice framework. It provides unified abstraction interfaces and multi-engine implementations for configuration centers, service discovery, logging systems, and transport layers.

Built with a **Lego-like composition design** — each plugin implements only the standard interfaces defined by the core framework. You can freely choose the underlying engine based on your tech stack, and switching engines requires no business code changes.

---

## Key Features

- **Unified Interfaces**: Six domains (Config / Registry / Log / Metrics / Transport / Tracer) with standard interfaces defined by the core framework
- **Multi-Engine Support**: 6 config centers, 8 registry providers, 6 logging backends, 3 metrics backends, 3 HTTP drivers, 1 OTLP tracing protocol, 12 message brokers — covering mainstream tech stacks
- **Zero Intrusion**: Business code depends only on interfaces, never on specific engine SDKs
- **Independent Versioning**: Each submodule has its own `go.mod`, import only what you need
- **Workspace Synergy**: Managed via `go.work` for a single-repo development experience

---

## Core Interfaces

### Config

| Interface | Methods | Description |
|-----------|---------|-------------|
| `Reader` | `Load(ctx, key) ([]byte, error)` | One-shot config loading by key |
| `Watcher` | `Watch(ctx, key) (<-chan struct{}, error)` | Signal-mode change notification |
| `ValueWatcher` | `WatchValue(ctx, key) (<-chan []byte, error)` | Push-mode change with value delivery |
| `Closer` | `Close() error` | Resource cleanup |
| `Decoder` | `Decode(data, out) error` | Raw bytes deserialization |

### Registry

| Interface | Methods | Description |
|-----------|---------|-------------|
| `Registrar` | `Register(ctx, *Instance)` / `Deregister(ctx, *Instance)` | Service registration lifecycle |
| `Discovery` | `GetService(ctx, name)` / `Watch(ctx, name)` | Service discovery and watching |
| `Watcher` | `Next(ctx) ([]*Instance, error)` / `Stop()` | Instance change stream |

### Log

| Interface | Methods | Description |
|-----------|---------|-------------|
| `Logger` | `Debug/Info/Warn/Error(ctx, msg, keyvals...)` | Four-level logging |
| `Logger` | `With(keyvals...) Logger` | Attach context fields |
| `Logger` | `Enabled(Level) bool` | Level filtering |

### Transport

| Interface | Methods | Description |
|-----------|---------|-------------|
| `Server` (HTTP) | `Handle / GET / POST / PUT / DELETE...` | Route registration |
| `Server` (HTTP) | `Start(ctx)` / `Stop(ctx)` / `Endpoint()` | Lifecycle management |
| `Driver` (HTTP) | `Handle / Start / Stop` | Framework adapter driver |

### Tracer

> Based on the OpenTelemetry standard. No custom interface — uses native OTel types directly.

| Type | Methods | Description |
|------|---------|-------------|
| `*sdktrace.TracerProvider` | `Tracer(name) trace.Tracer` | Create a standard OTel Tracer |
| `*sdktrace.TracerProvider` | `Shutdown(ctx)` | Shutdown provider, flush pending spans |
| `trace.Tracer` | `Start(ctx, name, opts...)` | Create span, inject trace context |

### Broker

| Interface | Methods | Description |
|------|------|------|
| `Broker` | `Name() string` | Get broker name |
| `Broker` | `Address() string` | Get broker address |
| `Broker` | `Init(...Option) error` | Initialize broker |
| `Broker` | `Connect() / Disconnect() error` | Connect / Disconnect |
| `Broker` | `Publish(ctx, topic, *Message, ...PublishOption) error` | Publish message to topic |
| `Broker` | `Subscribe(topic, Handler, Binder, ...SubscribeOption) (Subscriber, error)` | Subscribe to topic |
| `Broker` | `Request(ctx, topic, *Message, ...RequestOption) (*Message, error)` | Request-response pattern |
| `Message` | `Headers / Body / Key` | Message headers, body, partition key |
| `Event` | `Topic() / Message() / Ack() / Error()` | Event received by subscriber |
| `Subscriber` | `Unsubscribe() error` | Unsubscribe |

### Metrics

| Interface | Methods | Description |
|-----------|---------|-------------|
| `Metrics` | `Counter(ctx, name, value, labels)` | Monotonically increasing counter (requests, errors) |
| `Metrics` | `Histogram(ctx, name, value, labels)` | Distribution of observations (latency, payload size) |
| `Metrics` | `Gauge(ctx, name, value, labels)` | Point-in-time value (queue depth, active connections) |
| `Closer` | `Close() error` | Close and flush pending data |

### Encoding

| Interface/Func | Method | Description |
|------|------|------|
| `Codec` | `Marshal(v) ([]byte, error)` | Serialize |
| `Codec` | `Unmarshal(data, v) error` | Deserialize |
| `Codec` | `Name() string` | Codec name (json/proto/yaml) |
| Package funcs | `RegisterCodec(c)` / `GetCodec(name)` | Global registry |

---

### AI / LLM

> The three frameworks return incompatible types — no abstraction interface is defined.
> Only a shared config type `ai.Config` is provided.
>
> Each plugin constructor returns its framework's native type directly.

| Input | Constructor | Returns |
|-------|-------------|---------|
| `ai.Config` | `model.NewClient(cfg)` | `*openai.Client` |
| `ai.Config` | `eino.NewChatModel(ctx, cfg)` | `model.ChatModel` (Eino interface) |
| `ai.Config` | `langchaingo.NewModel(cfg)` | `llms.Model` (LangChainGo interface) |

## Plugin Matrix

### Config

| Plugin | Module Path | Engine |
|--------|------------|--------|
| Apollo | `github.com/tx7do/go-wind-plugins/config/apollo` | Ctrip Apollo |
| Consul | `github.com/tx7do/go-wind-plugins/config/consul` | HashiCorp Consul KV |
| Etcd | `github.com/tx7do/go-wind-plugins/config/etcd` | CoreOS etcd |
| Kubernetes | `github.com/tx7do/go-wind-plugins/config/kubernetes` | K8s ConfigMap / Secret |
| Nacos | `github.com/tx7do/go-wind-plugins/config/nacos` | Alibaba Nacos |
| Polaris | `github.com/tx7do/go-wind-plugins/config/polaris` | Tencent Polaris |
| Env | `github.com/tx7do/go-wind-plugins/config/env` | Environment variables |
| File | `github.com/tx7do/go-wind-plugins/config/file` | Local file |
| FS | `github.com/tx7do/go-wind-plugins/config/fs` | fs.FS |
| HTTP | `github.com/tx7do/go-wind-plugins/config/http` | HTTP remote fetch |
| OSS | `github.com/tx7do/go-wind-plugins/config/oss` | Object storage |
| Redis | `github.com/tx7do/go-wind-plugins/config/redis` | Redis KV |
| Vault | `github.com/tx7do/go-wind-plugins/config/vault` | HashiCorp Vault |
| Zookeeper | `github.com/tx7do/go-wind-plugins/config/zookeeper` | Apache ZooKeeper |

### Registry

| Plugin | Module Path | Engine |
|--------|------------|--------|
| Consul | `github.com/tx7do/go-wind-plugins/registry/consul` | HashiCorp Consul |
| Etcd | `github.com/tx7do/go-wind-plugins/registry/etcd` | CoreOS etcd |
| Eureka | `github.com/tx7do/go-wind-plugins/registry/eureka` | Netflix Eureka |
| Kubernetes | `github.com/tx7do/go-wind-plugins/registry/kubernetes` | K8s Endpoints |
| Nacos | `github.com/tx7do/go-wind-plugins/registry/nacos` | Alibaba Nacos |
| Polaris | `github.com/tx7do/go-wind-plugins/registry/polaris` | Tencent Polaris |
| ServiceComb | `github.com/tx7do/go-wind-plugins/registry/servicecomb` | Apache ServiceComb |
| Zookeeper | `github.com/tx7do/go-wind-plugins/registry/zookeeper` | Apache ZooKeeper |

### Log

| Plugin | Module Path | Engine |
|--------|------------|--------|
| Aliyun SLS | `github.com/tx7do/go-wind-plugins/log/aliyun` | Alibaba Cloud SLS |
| Tencent CLS | `github.com/tx7do/go-wind-plugins/log/tencent` | Tencent Cloud CLS |
| Fluent | `github.com/tx7do/go-wind-plugins/log/fluent` | Fluentd |
| Logrus | `github.com/tx7do/go-wind-plugins/log/logrus` | sirupsen/logrus |
| Zap | `github.com/tx7do/go-wind-plugins/log/zap` | uber-go/zap |
| Zerolog | `github.com/tx7do/go-wind-plugins/log/zerolog` | rs/zerolog |
| Charm | `github.com/tx7do/go-wind-plugins/log/charm` | charmbracelet/log |
| CloudWatch | `github.com/tx7do/go-wind-plugins/log/cloudwatch` | AWS CloudWatch Logs |
| Glog | `github.com/tx7do/go-wind-plugins/log/glog` | golang/glog |
| Hclog | `github.com/tx7do/go-wind-plugins/log/hclog` | hashicorp/go-hclog |
| Loki | `github.com/tx7do/go-wind-plugins/log/loki` | Grafana Loki |
| Phuslu | `github.com/tx7do/go-wind-plugins/log/phuslu` | phuslu/log |
| Sentry | `github.com/tx7do/go-wind-plugins/log/sentry` | getsentry/sentry-go |

### Transport

| Plugin | Module Path | Engine |
|--------|------------|--------|
| HTTP (stdlib) | `github.com/tx7do/go-wind-plugins/transport/http` | net/http |
| HTTP (Gin) | `github.com/tx7do/go-wind-plugins/transport/http/gin` | gin-gonic/gin |
| HTTP (Fiber) | `github.com/tx7do/go-wind-plugins/transport/http/fiber` | gofiber/fiber |
| gRPC | `github.com/tx7do/go-wind-plugins/transport/grpc` | google.golang.org/grpc |
| HTTP (Chi) | `github.com/tx7do/go-wind-plugins/transport/http/chi` | go-chi/chi |
| HTTP/3 | `github.com/tx7do/go-wind-plugins/transport/http3` | quic-go/http3 |
| WebSocket | `github.com/tx7do/go-wind-plugins/transport/websocket` | gorilla/websocket |
| Socket.IO | `github.com/tx7do/go-wind-plugins/transport/socketio` | googollee/go-socket.io |
| SignalR | `github.com/tx7do/go-wind-plugins/transport/signalr` | SignalR protocol |
| SSE | `github.com/tx7do/go-wind-plugins/transport/sse` | Server-Sent Events |
| TCP | `github.com/tx7do/go-wind-plugins/transport/tcp` | net.Listener |
| KCP | `github.com/tx7do/go-wind-plugins/transport/kcp` | xtaci/kcp-go |
| WebRTC | `github.com/tx7do/go-wind-plugins/transport/webrtc` | pion/webrtc v4 |
| WebTransport | `github.com/tx7do/go-wind-plugins/transport/webtransport` | webtransport-go |
| GraphQL | `github.com/tx7do/go-wind-plugins/transport/graphql` | graphql-go |
| Thrift | `github.com/tx7do/go-wind-plugins/transport/thrift` | Apache Thrift |
| tRPC | `github.com/tx7do/go-wind-plugins/transport/trpc` | tRPC protocol |
| Cron | `github.com/tx7do/go-wind-plugins/transport/cron` | robfig/cron |
| HPTimer | `github.com/tx7do/go-wind-plugins/transport/hptimer` | High-precision timer |
| Asynq | `github.com/tx7do/go-wind-plugins/transport/asynq` | hibiken/asynq |
| Machinery | `github.com/tx7do/go-wind-plugins/transport/machinery` | machinery (Celery-like) |
| MCP | `github.com/tx7do/go-wind-plugins/transport/mcp` | Model Context Protocol |

### Tracer

| Plugin | Module Path | Engine |
|--------|------------|--------|
| OTLP | `github.com/tx7do/go-wind-plugins/tracer/otlp` | OpenTelemetry Protocol (OTLP) |

**Note**: OTLP is the standard protocol of OpenTelemetry, supporting all major backends: Jaeger, Zipkin, SkyWalking, Tempo (Grafana), Datadog, Alibaba Cloud ARMS, Tencent Cloud APM, etc. Just configure the endpoint to switch backends without changing plugins.

### Metrics

| Plugin | Module Path | Engine |
|--------|------------|--------|
| Prometheus | `github.com/tx7do/go-wind-plugins/metrics/prometheus` | Prometheus client_golang |
| OpenTelemetry | `github.com/tx7do/go-wind-plugins/metrics/otel` | OTLP (gRPC/HTTP) |
| Datadog | `github.com/tx7do/go-wind-plugins/metrics/datadog` | DogStatsD |

### Encoding

| Plugin | Module Path | Engine |
|--------|------------|--------|
| JSON | `github.com/tx7do/go-wind-plugins/encoding/json` | encoding/json |
| Protobuf | `github.com/tx7do/go-wind-plugins/encoding/proto` | google.golang.org/protobuf |
| YAML | `github.com/tx7do/go-wind-plugins/encoding/yaml` | gopkg.in/yaml.v3 |
| TOML | `github.com/tx7do/go-wind-plugins/encoding/toml` | pelletier/go-toml |
| XML | `github.com/tx7do/go-wind-plugins/encoding/xml` | encoding/xml |
| MsgPack | `github.com/tx7do/go-wind-plugins/encoding/msgpack` | vmihailenco/msgpack |
| Avro | `github.com/tx7do/go-wind-plugins/encoding/avro` | linkedin/goavro |
| BSON | `github.com/tx7do/go-wind-plugins/encoding/bson` | go.mongodb.org/mongo-driver |
| CBOR | `github.com/tx7do/go-wind-plugins/encoding/cbor` | fxamacker/cbor |
| FlatBuffers | `github.com/tx7do/go-wind-plugins/encoding/flatbuffers` | google/flatbuffers |
| Gob | `github.com/tx7do/go-wind-plugins/encoding/gob` | encoding/gob |
| Thrift | `github.com/tx7do/go-wind-plugins/encoding/thrift` | Apache Thrift |

### Workflow

> The four engines have incompatible workflow operation parameters and return types. Only a minimal common interface `workflow.Client` (`Close() error`) is extracted.

| Plugin | Module Path | Framework |
|--------|------------|-----------|
| Argo Workflows | `github.com/tx7do/go-wind-plugins/workflow/argo` | Argo Workflows REST API |
| Conductor | `github.com/tx7do/go-wind-plugins/workflow/conductor` | conductor-sdk/conductor-go |
| GoWorkflows | `github.com/tx7do/go-wind-plugins/workflow/goworkflows` | cschleiden/go-workflows |
| Temporal | `github.com/tx7do/go-wind-plugins/workflow/temporal` | temporal.io/sdk |

### Object Storage

> The two OSS implementations have incompatible SDKs and return types. Each defines its own local `Config`; no shared interface is extracted.

| Plugin | Module Path | Framework |
|--------|------------|-----------|
| MinIO | `github.com/tx7do/go-wind-plugins/oss/minio` | minio/minio-go |
| S3 | `github.com/tx7do/go-wind-plugins/oss/s3` | aws/aws-sdk-go-v2 |

### AI / LLM

> Each AI submodule defines its own configuration and client, supporting different large language model services.

| Plugin | Module Path | Framework |
|--------|------------|-----------|
| OpenAI Client | `github.com/tx7do/go-wind-plugins/ai/openai` | sashabaranov/go-openai |
| Eino | `github.com/tx7do/go-wind-plugins/ai/eino` | cloudwego/eino |
| LangChainGo | `github.com/tx7do/go-wind-plugins/ai/langchaingo` | tmc/langchaingo |

### Cache

| Plugin | Module Path | Engine |
|--------|------------|--------|
| Local | `github.com/tx7do/go-wind-plugins/cache/local` | Local in-memory cache |
| Redis | `github.com/tx7do/go-wind-plugins/cache/redis` | gomodule/redigo |

### Circuit Breaker

| Plugin | Module Path | Engine |
|--------|------------|--------|
| Hystrix | `github.com/tx7do/go-wind-plugins/circuitbreaker/hystrix` | afex/hystrix-go |
| Sentinel | `github.com/tx7do/go-wind-plugins/circuitbreaker/sentinel` | alibaba/sentinel-golang |
| SRE | `github.com/tx7do/go-wind-plugins/circuitbreaker/sres` | SRE adaptive circuit breaking |
| Vegas | `github.com/tx7do/go-wind-plugins/circuitbreaker/vegas` | Vegas adaptive rate limiting |

### Rate Limiter

| Plugin | Module Path | Engine |
|--------|------------|--------|
| Token Bucket | `github.com/tx7do/go-wind-plugins/ratelimit/tokenbucket` | Token bucket algorithm |
| BBR | `github.com/tx7do/go-wind-plugins/ratelimit/bbr` | BBR adaptive rate limiting |
| Sentinel | `github.com/tx7do/go-wind-plugins/ratelimit/sentinel` | alibaba/sentinel-golang |

### Security

| Plugin | Module Path | Engine |
|--------|------------|--------|
| JWT | `github.com/tx7do/go-wind-plugins/security/authn` | golang-jwt/jwt |
| Casbin | `github.com/tx7do/go-wind-plugins/security/authz` | casbin/casbin |
| Crypto | `github.com/tx7do/go-wind-plugins/security/crypto` | go-utils/crypto |

### Miscellaneous Tool Modules

| Module | Path | Description |
|------|------|------|
| Errors | `github.com/tx7do/go-wind-plugins/errors` | Unified error codes and error types |
| Health | `github.com/tx7do/go-wind-plugins/health` | HTTP health check |
| Pprof | `github.com/tx7do/go-wind-plugins/pprof` | Profiling endpoint |
| Retry | `github.com/tx7do/go-wind-plugins/retry` | Retry policy (exponential backoff) |

### Broker

> Each Broker engine has its own SDK and configuration options, but all implement the core `broker.Broker` interface.

| Plugin | Module Path | Engine |
|--------|------------|--------|
| Kafka | `github.com/tx7do/go-wind-plugins/broker/kafka` | segmentio/kafka-go |
| RabbitMQ | `github.com/tx7do/go-wind-plugins/broker/rabbitmq` | rabbitmq/amqp091-go |
| NATS | `github.com/tx7do/go-wind-plugins/broker/nats` | nats-io/nats.go |
| MQTT | `github.com/tx7do/go-wind-plugins/broker/mqtt` | eclipse/paho.mqtt.golang |
| Pulsar | `github.com/tx7do/go-wind-plugins/broker/pulsar` | apache/pulsar-client-go |
| Redis | `github.com/tx7do/go-wind-plugins/broker/redis` | gomodule/redigo |
| RocketMQ | `github.com/tx7do/go-wind-plugins/broker/rocketmq` | apache/rocketmq-client-go + rocketmq-clients |
| NSQ | `github.com/tx7do/go-wind-plugins/broker/nsq` | nsqio/go-nsq |
| SQS | `github.com/tx7do/go-wind-plugins/broker/sqs` | aws/aws-sdk-go-v2 |
| GCP PubSub | `github.com/tx7do/go-wind-plugins/broker/gcpubsub` | cloud.google.com/go/pubsub |
| Azure Service Bus | `github.com/tx7do/go-wind-plugins/broker/azuresb` | azure-sdk-for-go |
| STOMP | `github.com/tx7do/go-wind-plugins/broker/stomp` | go-stomp/stomp |

---

## Architecture

```mermaid
graph TB
    App["Application Layer<br/>Business code depends only on interfaces"]
    Core["go-wind Core Framework<br/>Defines standard interfaces + wind.Instance"]

    subgraph Config["Config"]
        CApollo[Apollo]
        CConsul[Consul]
        CEtcd[etcd]
        CEnv[Env]
        CFile[File]
        CFS[FS]
        CHTTP[HTTP]
        CK8s[Kubernetes]
        CNacos[Nacos]
        COSS[OSS]
        CPolaris[Polaris]
        CRedis[Redis]
        CVault[Vault]
        CZK[ZooKeeper]
    end

    subgraph Registry["Registry"]
        RConsul[Consul]
        REtcd[etcd]
        REureka[Eureka]
        RK8s[Kubernetes]
        RNacos[Nacos]
        RPolaris[Polaris]
        RServiceComb[ServiceComb]
        RZK[ZooKeeper]
    end

    subgraph Log["Log"]
        LAliyun[Aliyun SLS]
        LCharm[Charm]
        LCloudWatch[CloudWatch]
        LFluent[Fluent]
        LGlog[Glog]
        LHclog[Hclog]
        LLogrus[Logrus]
        LLoki[Loki]
        LPhuslu[Phuslu]
        LSentry[Sentry]
        LTencent[Tencent CLS]
        LZap[Zap]
        LZerolog[Zerolog]
    end

    subgraph Transport["Transport"]
        THTTP[HTTP]
        TChi[Chi]
        TGin[Gin]
        TFiber[Fiber]
        THTTP3[HTTP/3]
        TGRPC[gRPC]
        TWS[WebSocket]
        TSIO[Socket.IO]
        TSignalR[SignalR]
        TSSE[SSE]
        TTCP[TCP]
        TKCP[KCP]
        TWebRTC[WebRTC]
        TWT[WebTransport]
        TGraphQL[GraphQL]
        TThrift[Thrift]
        TTRPC[tRPC]
        TCron[Cron]
        THPTimer[HPTimer]
        TAsynq[Asynq]
        TMachinery[Machinery]
        TMCP[MCP]
    end

    subgraph Tracer["Tracing"]
        TOTLP[OTLP]
    end

    subgraph Metrics["Metrics"]
        MProm[Prometheus]
        MOtel[OTLP]
        MDatadog[Datadog]
    end

    subgraph Workflow["Workflow"]
        WArgo[Argo]
        WConductor[Conductor]
        WGoWorkflows[GoWorkflows]
        WTemporal[Temporal]
    end

    subgraph OSS["Object Storage"]
        OMinio[MinIO]
        OS3[S3]
    end

    subgraph Broker["Broker"]
        BKafka[Kafka]
        BRabbitMQ[RabbitMQ]
        BNATS[NATS]
        BMQTT[MQTT]
        BPulsar[Pulsar]
        BRedis[Redis]
        BRocketMQ[RocketMQ]
        BNSQ[NSQ]
        BSQS[SQS]
        BGCP[GCP PubSub]
        BAzure[Azure SB]
        BActiveMQ[ActiveMQ]
        BSTOMP[STOMP]
    end

    subgraph Encoding["Encoding"]
        EJSON[JSON]
        EProto[Protobuf]
        EYAML[YAML]
        ETOML[TOML]
        EXML[XML]
        EMsgPack[MsgPack]
        EAvro[Avro]
        EBSON[BSON]
        EFlat[FlatBuffers]
    end

    subgraph AI["AI / LLM"]
        AOpenAI[OpenAI]
        ALangChain[LangChainGo]
        AEino[Eino]
    end

    subgraph Cache["Cache"]
        CacheLocal[Local]
        CacheRedis[Redis]
    end

    subgraph Security["Security"]
        SAuthN[JWT]
        SAuthZ[Casbin]
        SCrypto[Crypto]
    end

    subgraph Resilience["Resilience"]
        RCBHystrix[Hystrix]
        RCBSentinel[Sentinel]
        RCBRatelimit[Token Bucket]
        RCBBBR[BBR]
    end

    App --> Core
    Core --> Config
    Core --> Registry
    Core --> Log
    Core --> Transport
    Core --> Tracer
    Core --> Metrics
    Core --> Workflow
    Core --> OSS
    Core --> Broker
    Core --> Encoding
    Core --> AI
    Core --> Cache
    Core --> Security
    Core --> Resilience
```

---

## Project Structure

```
go-wind-plugins/
├── ai/                             # AI / LLM plugins (independent self-contained modules)
│   ├── openai/                     # OpenAI-compatible client (sashabaranov/go-openai)
│   │   ├── client.go               # Returns *openai.Client
│   │   ├── config.go               # Local Config types
│   │   └── options.go              # HTTP client options
│   ├── langchaingo/                # LangChainGo (tmc/langchaingo)
│   │   ├── client.go               # Returns llms.Model
│   │   ├── config.go               # Local Config types
│   │   ├── agent.go                # Agent / Executor helpers
│   │   ├── chain.go                # Chain helpers
│   │   ├── memory.go               # Memory helpers
│   │   ├── embedding.go            # Embedding helpers
│   │   ├── vectorstore.go          # VectorStore helpers
│   │   └── options.go              # OpenAI/Ollama/HTTP options
│   └── eino/                       # ByteDance Eino framework (cloudwego/eino)
│       ├── client.go               # Returns model.ChatModel
│       ├── config.go               # Local Config types
│       ├── compose.go              # Chain/Graph/Workflow helpers
│       ├── chain.go                # Chain node append methods
│       ├── prompt.go               # ChatTemplate helpers
│       ├── tool.go                 # Tool node helpers
│       └── options.go              # ChatModel config modifier
├── broker/                         # Message broker interfaces and plugins
│   ├── broker.go                   # Broker interface (Publish/Subscribe/Request)
│   ├── kafka/                      # Apache Kafka (segmentio/kafka-go)
│   ├── rabbitmq/                   # RabbitMQ (rabbitmq/amqp091-go)
│   ├── nats/                       # NATS JetStream (nats-io/nats.go)
│   ├── mqtt/                       # MQTT (eclipse/paho.mqtt.golang)
│   ├── pulsar/                     # Apache Pulsar (apache/pulsar-client-go)
│   ├── redis/                      # Redis Pub/Sub (gomodule/redigo)
│   ├── rocketmq/                   # Apache RocketMQ (dual SDK support)
│   ├── nsq/                        # NSQ (nsqio/go-nsq)
│   ├── sqs/                        # AWS SQS (aws/aws-sdk-go-v2)
│   ├── gcpubsub/                   # Google Cloud Pub/Sub
│   ├── azuresb/                    # Azure Service Bus
│   ├── stomp/                      # STOMP protocol
│   ├── message.go                  # Message struct (Headers/Body/Key)
│   ├── event.go                    # Event interface (Topic/Message/Ack)
│   ├── options.go                  # Broker configuration options
│   ├── subscriber.go               # Subscriber management (SubscriberSyncMap)
│   ├── encoding.go                 # Message encoding integration
│   ├── publish.go                  # Publish middleware chain
│   └── typed_handler.go            # Generic TypedHandler support
├── cache/                          # Cache interfaces and plugins
│   ├── cache.go                    # Cache interface definition
│   ├── local/                      # Local in-memory cache
│   └── redis/                      # Redis cache
├── circuitbreaker/                 # Circuit breaker interfaces and plugins
│   ├── circuitbreaker.go           # CircuitBreaker interface definition
│   ├── hystrix/                    # Hystrix
│   ├── sentinel/                   # Sentinel
│   ├── sres/                       # SRE adaptive circuit breaking
│   └── vegas/                      # Vegas adaptive rate limiting
├── config/                         # Config center interfaces and plugins
│   ├── config.go                   # Standard interfaces (Reader/Watcher/ValueWatcher...)
│   ├── apollo/                     # Ctrip Apollo
│   ├── consul/                     # HashiCorp Consul KV
│   ├── etcd/                       # CoreOS etcd
│   ├── env/                        # Environment variables
│   ├── file/                       # Local file
│   ├── fs/                         # fs.FS
│   ├── http/                       # HTTP remote fetch
│   ├── kubernetes/                 # Kubernetes ConfigMap/Secret
│   ├── nacos/                      # Alibaba Nacos
│   ├── oss/                        # Object storage
│   ├── polaris/                    # Tencent Polaris
│   ├── redis/                      # Redis KV
│   ├── vault/                      # HashiCorp Vault
│   └── zookeeper/                  # Apache ZooKeeper
├── encoding/                       # Encoding interfaces and plugins
│   ├── encoding.go                 # Codec interface definition + registry
│   ├── json/                       # JSON codec (encoding/json)
│   │   └── json.go
│   ├── proto/                      # Protobuf codec (google.golang.org/protobuf)
│   │   └── proto.go
│   ├── yaml/                       # YAML codec (gopkg.in/yaml.v3)
│   │   └── yaml.go
│   ├── toml/                       # TOML
│   ├── xml/                        # XML
│   ├── msgpack/                    # MessagePack
│   ├── avro/                       # Avro
│   ├── bson/                       # BSON
│   ├── cbor/                       # CBOR
│   ├── flatbuffers/                # FlatBuffers
│   ├── gob/                        # Gob
│   └── thrift/                     # Apache Thrift
├── errors/                         # Unified error codes and error types
├── health/                         # HTTP health check
├── log/                            # Logging interfaces and adapters
│   ├── slog_logger.go              # stdlib slog adapter (default)
│   ├── aliyun/                     # Alibaba Cloud SLS
│   ├── charm/                      # Charm
│   ├── cloudwatch/                 # AWS CloudWatch
│   ├── fluent/                     # Fluentd
│   ├── glog/                       # glog
│   ├── hclog/                      # hclog
│   ├── logrus/                     # sirupsen/logrus
│   ├── loki/                       # Grafana Loki
│   ├── phuslu/                     # phuslu/log
│   ├── sentry/                     # Sentry
│   ├── tencent/                    # Tencent Cloud CLS
│   ├── zap/                        # uber-go/zap
│   ├── zerolog/                    # rs/zerolog
│   ├── level_filter.go             # Level filter
│   └── multi_logger.go             # Multi-logger
├── metrics/                        # Metrics interfaces and plugins
│   ├── prometheus/                 # Prometheus client_golang implementation
│   │   └── prometheus.go           # Prometheus provider
│   ├── otel/                       # OpenTelemetry OTLP implementation
│   │   └── otel.go                 # OTLP metric exporter configuration
│   ├── datadog/                    # Datadog DogStatsD implementation
│   │   └── datadog.go              # DogStatsD provider
│   ├── metrics.go                  # Metrics interface (Counter/Histogram/Gauge)
│   └── doc.go                      # Package documentation
├── oss/                            # Object storage plugins (self-contained config)
│   ├── minio/                      # MinIO (minio/minio-go)
│   │   ├── client.go               # Returns *minio.Client
│   │   └── config.go               # Local Config types
│   └── s3/                         # AWS S3 compatible (aws-sdk-go-v2)
│       ├── client.go               # Returns *s3.Client
│       ├── storage.go              # Storage wrapper (default bucket)
│       ├── config.go               # Local Config types
│       └── errors.go               # Sentinel errors
├── pprof/                          # Profiling endpoint
├── ratelimit/                      # Rate limiter interfaces and plugins
│   ├── tokenbucket/                # Token bucket
│   ├── bbr/                        # BBR
│   └── sentinel/                   # Sentinel
├── registry/                       # Service discovery interfaces and plugins
│   ├── consul/                     # HashiCorp Consul
│   ├── etcd/                       # CoreOS etcd
│   ├── eureka/                     # Netflix Eureka
│   ├── kubernetes/                 # Kubernetes Endpoints
│   ├── nacos/                      # Alibaba Nacos
│   ├── polaris/                    # Tencent Polaris
│   ├── servicecomb/                # Apache ServiceComb
│   ├── zookeeper/                  # Apache ZooKeeper
│   ├── registrar.go                # Registrar interface
│   └── discovery.go                # Discovery / Watcher interfaces
├── retry/                          # Retry policies
├── security/                       # Security module
│   ├── authn/                      # Authentication (JWT/OAuth2/OIDC)
│   ├── authz/                      # Authorization (Casbin)
│   └── crypto/                     # Encryption / decryption
├── tracer/                         # Distributed tracing plugins
│   └── otlp/                       # OpenTelemetry Protocol (OTLP) implementation
│       └── otlp.go                 # Returns native *sdktrace.TracerProvider
├── transport/                      # Transport layer interfaces and drivers
│   ├── http/                       # HTTP Server + Driver interface + default driver
│   │   ├── server.go               # Server impl (routing/middleware/TLS)
│   │   └── options.go              # Configuration options
│   ├── http3/                      # HTTP/3 (QUIC)
│   ├── grpc/                       # gRPC Server
│   ├── websocket/                  # WebSocket
│   ├── socketio/                   # Socket.IO
│   ├── signalr/                    # SignalR
│   ├── sse/                        # Server-Sent Events
│   ├── tcp/                        # TCP Socket
│   ├── kcp/                        # KCP (UDP)
│   ├── webrtc/                     # WebRTC SFU
│   ├── webtransport/               # WebTransport
│   ├── graphql/                    # GraphQL
│   ├── thrift/                     # Apache Thrift
│   ├── trpc/                       # tRPC
│   ├── cron/                       # Cron scheduled jobs
│   ├── hptimer/                    # High-precision timer
│   ├── asynq/                      # Asynq async task queue
│   ├── machinery/                  # Machinery (Celery-like)
│   └── mcp/                        # Model Context Protocol
├── workflow/                       # Workflow engine plugins (defines Client/Worker interfaces)
│   ├── argo/                       # Argo Workflows (REST API)
│   │   ├── client.go               # Submit/Get/Suspend/Resume/Terminate
│   │   ├── options.go              # Config options + Argo type definitions
│   │   └── logger.go               # slog logging wrapper
│   ├── conductor/                  # Netflix Conductor (conductor-go SDK)
│   │   ├── client.go               # Start/Get/Pause/Resume/Terminate
│   │   ├── worker.go               # Task Worker
│   │   ├── options.go              # Config options
│   │   └── logger.go
│   ├── goworkflows/                # cschleiden/go-workflows
│   │   ├── client.go               # Create/Cancel/Signal/Wait
│   │   ├── worker.go               # Workflow + Activity Worker
│   │   ├── options.go              # Worker options
│   │   └── logger.go
│   ├── temporal/                   # Temporal (temporal.io/sdk)
│   │   ├── client.go               # Execute/Signal/Query/Cancel (native OTel tracing)
│   │   ├── worker.go               # Worker + built-in message processing Activity
│   │   ├── workflow.go             # Built-in BrokerMessageWorkflow
│   │   ├── options.go              # Config options
│   │   └── logger.go
│   └── workflow.go                 # Common interfaces (Client/Worker)
├── go.work                         # Go Workspace multi-module management
├── LICENSE
└── README.md
```

---

## Quick Start

### Installation

```bash
# Import only what you need, e.g. etcd config + nacos registry
go get github.com/tx7do/go-wind-plugins/config/etcd
go get github.com/tx7do/go-wind-plugins/registry/nacos
go get github.com/tx7do/go-wind-plugins/log/zap
```

### Config Example (etcd)

```go
package main

import (
    "context"
    "fmt"

    clientv3 "go.etcd.io/etcd/client/v3"

    "github.com/tx7do/go-wind-plugins/config/etcd"
)

func main() {
    client, err := clientv3.New(clientv3.Config{
        Endpoints: []string{"localhost:2379"},
    })
    if err != nil {
        panic(err)
    }

    cfg, err := etcd.New(client)
    if err != nil {
        panic(err)
    }

    // Load config
    data, err := cfg.Load(context.Background(), "/myapp/config")
    if err != nil {
        panic(err)
    }
    fmt.Println("config:", string(data))

    // Watch config changes
    ch, _ := cfg.WatchValue(context.Background(), "/myapp/config")
    for val := range ch {
        fmt.Println("config updated:", string(val))
    }
}
```

### Registry Example (nacos)

```go
package main

import (
    "context"
    "fmt"

    "github.com/nacos-group/nacos-sdk-go/v2/clients"
    "github.com/nacos-group/nacos-sdk-go/v2/common/constant"
    "github.com/nacos-group/nacos-sdk-go/v2/vo"
    wind "github.com/tx7do/go-wind"

    "github.com/tx7do/go-wind-plugins/registry/nacos"
)

func main() {
    client, _ := clients.NewNamingClient(vo.NacosClientParam{
        ServerConfigs: []constant.ServerConfig{
            {IpAddr: "127.0.0.1", Port: 8848},
        },
        ClientConfig: &constant.ClientConfig{
            NamespaceId: "public",
        },
    })

    r := nacos.New(client)

    // Register service
    instance := &wind.Instance{
        Name:      "my-service",
        Version:   "v1.0.0",
        Endpoints: []string{"grpc://127.0.0.1:8080"},
    }
    _ = r.Register(context.Background(), instance)

    // Discover services
    services, _ := r.GetService(context.Background(), "my-service.grpc")
    for _, svc := range services {
        fmt.Printf("found: %+v\n", svc)
    }
}
```

### HTTP Server Example (Gin driver)

```go
package main

import (
    "context"
    "net/http"

    httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
    "github.com/tx7do/go-wind-plugins/transport/http/gin"
)

func main() {
    srv := httpPlugin.NewServer(":8080",
        httpPlugin.WithDriver(gin.NewDriver()),
        httpPlugin.WithMiddleware(func(next http.Handler) http.Handler {
            return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                w.Header().Set("X-Engine", "gin")
                next.ServeHTTP(w, r)
            })
        }),
    )

    srv.GET("/", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("Hello from Gin driver!"))
    })

    srv.Start(context.Background())
}
```

### Swagger UI Documentation

Mount an embedded Swagger UI on the HTTP server. Three documentation sources are supported: remote URL, local file, and in-memory data.

```go
package main

import (
    "context"

    httpServer "github.com/tx7do/go-wind-plugins/transport/http"
    "github.com/tx7do/go-wind-plugins/transport/http/driver/std"
    "github.com/tx7do/go-wind-plugins/transport/http/swagger"
)

func main() {
    srv := httpServer.NewServer(":8080",
        httpServer.WithDriver(std.NewDriver()),
    )

    // Option 1: remote URL, the frontend fetches openapi.json directly
    swagger.Register(srv,
        swagger.WithTitle("Petstore"),
        swagger.WithRemoteFileURL("https://petstore3.swagger.io/api/v3/openapi.json"),
        swagger.WithBasePath("/docs/"),
    )

    // Option 2: local file, the server reads and hosts it
    // swagger.Register(srv,
    //     swagger.WithTitle("API"),
    //     swagger.WithLocalFile("./openapi.json"),
    //     swagger.WithBasePath("/docs/"),
    // )

    // Option 3: in-memory data, hosting dynamically generated documentation
    // data := generateOpenAPI()
    // swagger.Register(srv,
    //     swagger.WithTitle("API"),
    //     swagger.WithMemoryData(data, "json"),
    //     swagger.WithBasePath("/docs/"),
    // )

    srv.Start(context.Background())
}
```

Visit `http://localhost:8080/docs/` to view the interactive API documentation.

### ReDoc Documentation

ReDoc, the alternative documentation rendering engine, likewise supports two documentation sources: remote URL and local file.

```go
package main

import (
    "context"

    httpServer "github.com/tx7do/go-wind-plugins/transport/http"
    "github.com/tx7do/go-wind-plugins/transport/http/driver/std"
    "github.com/tx7do/go-wind-plugins/transport/http/redoc"
)

func main() {
    srv := httpServer.NewServer(":8080",
        httpServer.WithDriver(std.NewDriver()),
    )

    // Remote URL mode
    redoc.Register(srv,
        redoc.WithTitle("Petstore"),
        redoc.WithDescription("A sample API powered by ReDoc"),
        redoc.WithRemoteFileURL("https://petstore3.swagger.io/api/v3/openapi.json"),
        redoc.WithBasePath("/redoc/"),
    )

    // Local file mode
    // redoc.Register(srv,
    //     redoc.WithTitle("Petstore"),
    //     redoc.WithLocalFile("./openapi.json"),
    //     redoc.WithBasePath("/redoc/"),
    // )

    srv.Start(context.Background())
}
```

Visit `http://localhost:8080/redoc/` to view the interactive documentation rendered by ReDoc.

### Logging Example (Zap)

```go
package main

import (
    "context"
    "github.com/tx7do/go-wind-plugins/log/zap"
)

func main() {
    logger, _ := zap.NewZapLogger()
    logger.Info(context.Background(), "service started", "port", 8080)
    logger.With("module", "auth").Error(context.Background(), "token expired")
}
```

### Distributed Tracing Example (OTLP)

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/tx7do/go-wind-plugins/tracer/otlp"
)

func main() {
    // Create OTLP TracerProvider (auto-registers as global TracerProvider)
    tp, err := otlp.New(
        otlp.WithEndpoint("localhost:4317"),     // OTLP collector endpoint
        otlp.WithServiceName("my-service"),      // Service name
        otlp.WithServiceVersion("v1.0.0"),       // Service version
        otlp.WithSampleRatio(1.0),               // Full sampling
        otlp.WithInsecure(true),                 // Disable TLS
    )
    if err != nil {
        panic(err)
    }
    defer tp.Shutdown(context.Background())

    // Use the standard OpenTelemetry API to create a tracer
    tracer := tp.Tracer("my-service")

    // Create span
    ctx, span := tracer.Start(context.Background(), "handle-request")
    defer span.End()

    // Simulate business logic
    time.Sleep(100 * time.Millisecond)
    fmt.Println("Request processed")

    // Nested span
    _, childSpan := tracer.Start(ctx, "database-query")
    defer childSpan.End()
    time.Sleep(50 * time.Millisecond)
    fmt.Println("Query completed")
}
```

**Prerequisite**: Start an OTLP collector first, e.g., using Jaeger:

```bash
docker run -d --name jaeger \
  -e COLLECTOR_OTLP_ENABLED=true \
  -p 4317:4317 \
  -p 16686:16686 \
  jaegertracing/jaeger:latest
```

Then visit http://localhost:16686 to view traces.

### Metrics Example (Prometheus)

```go
package main

import (
    "context"
    "log"
    "net/http"

    "github.com/prometheus/client_golang/promhttp"

    "github.com/tx7do/go-wind-plugins/metrics/prometheus"
)

func main() {
    // Create Prometheus metrics provider
    m, err := prometheus.NewWithDefaultRegistry(
        prometheus.WithNamespace("myapp"),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Record metrics
    ctx := context.Background()
    m.Counter(ctx, "requests_total", 1, map[string]string{"method": "GET"})
    m.Histogram(ctx, "request_duration_seconds", 0.042, map[string]string{"method": "GET"})
    m.Gauge(ctx, "queue_depth", 42, map[string]string{"queue": "email"})

    // Expose /metrics endpoint for Prometheus scraping
    http.Handle("/metrics", promhttp.Handler())
    log.Println("metrics on :9090/metrics")
    log.Fatal(http.ListenAndServe(":9090", nil))
}
```

### Broker Example (Kafka)

```go
package main

import (
    "context"
    "fmt"
    "log/slog"

    "github.com/tx7do/go-wind-plugins/broker"
    kafkaBroker "github.com/tx7do/go-wind-plugins/broker/kafka"
)

func main() {
    // Create Kafka broker
    b := kafkaBroker.NewBroker(
        broker.WithAddress("localhost:9092"),
        broker.WithCodec("json"),
    )

    if err := b.Init(); err != nil {
        panic(err)
    }
    if err := b.Connect(); err != nil {
        panic(err)
    }
    defer b.Disconnect()

    // Publish
    ctx := context.Background()
    msg := map[string]any{"temperature": 25.5, "humidity": 60.0}
    err := b.Publish(ctx, "sensor.temperature",
        broker.NewMessage(msg,
            broker.WithPublishHeaders(map[string]string{"version": "1.0"}),
        ),
    )
    if err != nil {
        slog.Error("publish failed", "error", err)
    }
    fmt.Println("message published")

    // Subscribe
    _, err = b.Subscribe("sensor.temperature",
        func(ctx context.Context, event broker.Event) error {
            slog.Info("received",
                "topic", event.Topic(),
                "body", fmt.Sprintf("%v", event.Message().Body),
            )
            return nil
        },
        func() any { return &map[string]any{} },
    )
    if err != nil {
        panic(err)
    }

    select {} // Block and wait for messages
}
```

### AI / LLM Example (LangChainGo)

```go
package main

import (
    "context"
    "fmt"

    "github.com/tx7do/go-wind-plugins/ai"
    "github.com/tx7do/go-wind-plugins/ai/langchaingo"
)

func main() {
    cfg := &ai.Config{
        Type:      ai.ModelTypeCloud,
        ModelName: "gpt-4o",
        Cloud: &ai.CloudConfig{
            ApiKey:  "sk-xxx",
            BaseUrl: "https://api.openai.com/v1",
        },
        TimeoutSeconds: 60,
    }

    llm, err := langchaingo.NewModel(cfg)
    if err != nil {
        panic(err)
    }

    resp, err := llm.Call(context.Background(),
        "Explain microservices in one sentence",
    )
    if err != nil {
        panic(err)
    }
    fmt.Println(resp)
}
```

---
## Design Philosophy

### Lego-Style Composition

go-wind-plugins follows the principle of **interfaces first, implementations optional**:

1. **Core framework defines interfaces**: `go-wind` defines `Reader`, `Registrar`, `Logger`, `Server` and other standard interfaces
2. **Plugins implement interfaces**: Each plugin module implements only the corresponding standard interface
3. **Application-layer injection**: Business code references plugins through interfaces; switching engines is just an import change

### Independent Versioning

Each submodule has its own `go.mod` and can be versioned independently:

```
github.com/tx7do/go-wind-plugins/config        # Interface definitions
github.com/tx7do/go-wind-plugins/config/etcd    # etcd implementation
github.com/tx7do/go-wind-plugins/registry       # Interface definitions
github.com/tx7do/go-wind-plugins/registry/nacos # nacos implementation
```

---

## Contributing

Issues and Pull Requests are welcome!

1. Fork this repository
2. Create a feature branch: `git checkout -b feature/new-plugin`
3. Commit changes: `git commit -m 'feat: add new plugin'`
4. Push branch: `git push origin feature/new-plugin`
5. Submit a Pull Request

---

## License

[MIT License](LICENSE) © 2026 GoWind
