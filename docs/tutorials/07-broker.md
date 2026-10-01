# Go-Wind 插件教程 · 第 7 章：消息代理（Broker）

> 本篇是 [Go-Wind 插件系列教程](./README.md)的组成部分。消息代理插件实现 [`github.com/tx7do/go-wind-plugins/broker`](../../broker/) 根模块定义的 `Broker` 接口族。

## 1. 这一层解决什么问题

服务间异步通信：事件通知、任务分发、削峰填谷。各家 MQ 的概念模型差异极大（topic/queue/stream、pull/push、ack 语义各不相同），代理层把「发布」与「订阅」两个动作用统一接口盖住，具体的连接参数、序列化、错误恢复策略由各引擎插件自管。

## 2. 接口架构

| 接口 | 方法 | 说明 |
|------|------|------|
| `Broker` | `Name() string` / `Address() string` | 引擎标识与地址 |
| `Broker` | `Init(...Option) error` | 初始化 |
| `Broker` | `Connect() / Disconnect() error` | 建立 / 断开连接 |
| `Broker` | `Publish(ctx, topic, *Message, ...PublishOption) error` | 发布消息到主题 |
| `Broker` | `Subscribe(topic, Handler, Binder, ...SubscribeOption) (Subscriber, error)` | 订阅主题 |
| `Broker` | `Request(ctx, topic, *Message, ...RequestOption) (*Message, error)` | 请求-响应模式 |
| `Message` | `Headers / Body / Key` | 消息头、消息体、分区键 |
| `Event` | `Topic() / Message() / Ack() / Error()` | 订阅者收到的事件 |
| `Subscriber` | `Unsubscribe() error` | 取消订阅 |

模块另带消息封装、请求-响应封装、类型化 handler 辅助（`typed_handler.go`）等配套工具。`Ack` 语义依引擎而异——部分引擎是即时的，部分在消费位移提交后才生效，投递失败时的重投/死信行为以各引擎 README 为准。

## 3. 插件矩阵

> 各 Broker 引擎 SDK 差异较大，每个子模块独立定义配置选项，但均实现核心 `broker.Broker` 接口。

| 插件 | 模块路径 | 引擎 |
|------|---------|------|
| Kafka | `github.com/tx7do/go-wind-plugins/broker/kafka` | segmentio/kafka-go |
| RabbitMQ | `github.com/tx7do/go-wind-plugins/broker/rabbitmq` | rabbitmq/amqp091-go |
| NATS | `github.com/tx7do/go-wind-plugins/broker/nats` | nats-io/nats.go |
| MQTT | `github.com/tx7do/go-wind-plugins/broker/mqtt` | eclipse/paho.mqtt.golang |
| Pulsar | `github.com/tx7do/go-wind-plugins/broker/pulsar` | apache/pulsar-client-go |
| Redis | `github.com/tx7do/go-wind-plugins/broker/redis` | gomodule/redigo |
| RocketMQ | `github.com/tx7do/go-wind-plugins/broker/rocketmq` | apache/rocketmq-client-go + rocketmq-clients |
| NSQ | `github.com/tx7do/go-wind-plugins/broker/nsq` | nsqio/go-nsq |
| SQS | `github.com/tx7do/go-wind-plugins/broker/sqs` | aws/aws-sdk-go-v2 |
| Azure Service Bus | `github.com/tx7do/go-wind-plugins/broker/azuresb` | Azure Service Bus |
| GCP Pub/Sub | `github.com/tx7do/go-wind-plugins/broker/gcpubsub` | Google Cloud Pub/Sub |
| Stomp | `github.com/tx7do/go-wind-plugins/broker/stomp` | STOMP 协议 |

## 4. 两个使用面

- **库形态**：业务代码直接持有一个 `broker.Broker` 实例做发布订阅——适合工具、批处理、无服务器框架的场景。
- **传输层服务器形态**：`transport/kafka`、`transport/rabbitmq`、`transport/mqtt` 等服务器模块（见[第 8 章](./08-transport-middleware.md)）把「订阅主题」建模为一种服务器——消息到达即触发 handler，与 HTTP 请求 handler 的编程模型对齐，并同样接入[第 4 章](./04-metrics.md)所述的指标埋点。

选择哪个面取决于消费逻辑的位置：常驻服务用传输层形态获得统一生命周期管理；独立消费者/生产者工具用库形态。

## 5. 选型建议

| 需求形态 | 指向 |
|------|------|
| 高吞吐流式事件、日志管道 | Kafka / Pulsar（分区并行、持久化日志） |
| 任务队列、请求-响应 RPC over MQ | RabbitMQ / NATS（NATS 亦可做 subject 广播） |
| IoT 设备接入 | MQTT（QoS 等级、遗嘱消息） |
| 已在云厂商生态内 | SQS / Azure Service Bus / GCP Pub/Sub |
| 轻量、无独立设施 | Redis（注意持久化与内存约束） |

共同的工程红线：消费端 handler 必须幂等（至少一次投递是默认假设）；毒消息要有退避与死信出口；分区键选择决定顺序性与热点，按业务实体（如订单 ID）取键而非随机。

## 6. 深入阅读

各引擎模块 README（部署、配置项、语义细节）：[`kafka`](../../broker/kafka/README.md) · [`rabbitmq`](../../broker/rabbitmq/README.md) · [`nats`](../../broker/nats/README.md) · [`mqtt`](../../broker/mqtt/README.md) · [`pulsar`](../../broker/pulsar/README.md) · [`redis`](../../broker/redis/README.md) · [`rocketmq`](../../broker/rocketmq/README.md) · [`nsq`](../../broker/nsq/README.md) · [`sqs`](../../broker/sqs/README.md) · [`azuresb`](../../broker/azuresb/README.md) · [`gcpubsub`](../../broker/gcpubsub/README.md) · [`stomp`](../../broker/stomp/README.md)

另见 [根 README · 消息代理矩阵](../../README.md#消息代理broker)。
