# Kafka

Kafka是一个分布式流处理系统，流处理系统使它可以像消息队列一样publish或者subscribe消息，分布式提供了容错性，并发处理消息的机制。

## Kafka的基本概念

kafka运行在集群上，集群包含一个或多个服务器。kafka把消息存在topic中，每一条消息包含键值（key），值（value）和时间戳（timestamp）。

kafka有以下一些基本概念：

* **Producer** - 消息生产者，就是向kafka broker发消息的客户端。

* **Consumer** - 消息消费者，是消息的使用方，负责消费Kafka服务器上的消息。

* **Topic** - 主题，由用户定义并配置在Kafka服务器，用于建立Producer和Consumer之间的订阅关系。生产者发送消息到指定的Topic下，消息者从这个Topic下消费消息。

* **Partition** - 消息分区，一个topic可以分为多个 partition，每个partition是一个有序的队列。partition中的每条消息都会被分配一个有序的id（offset）。

* **Broker** - 一台kafka服务器就是一个broker。一个集群由多个broker组成。一个broker可以容纳多个topic。

* **Consumer Group** - 消费者分组，用于归组同类消费者。每个consumer属于一个特定的consumer group，多个消费者可以共同消息一个Topic下的消息，每个消费者消费其中的部分消息，这些消费者就组成了一个分组，拥有同一个分组名称，通常也被称为消费者集群。

* **Offset** - 消息在partition中的偏移量。每一条消息在partition都有唯一的偏移量，消息者可以指定偏移量来指定要消费的消息。

## Docker部署开发环境

```shell
docker pull soldevelo/kafka:latest

sudo chown -R 1001:1001 /root/app/kafka/

docker run -itd \
    --name kafka-standalone \
    --user root \
    -p 9092:9092 \
    -p 9093:9093 \
    -v /root/app/kafka:/kafka_data:/bitnami \
    -e KAFKA_ENABLE_KRAFT=yes \
    -e KAFKA_CFG_NODE_ID=1 \
    -e KAFKA_CFG_PROCESS_ROLES=broker,controller \
    -e KAFKA_CFG_CONTROLLER_LISTENER_NAMES=CONTROLLER \
    -e KAFKA_CFG_CONTROLLER_QUORUM_VOTERS=1@127.0.0.1:9093 \
    -e KAFKA_CFG_LISTENER_SECURITY_PROTOCOL_MAP=CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT \
    -e KAFKA_CFG_LISTENERS=PLAINTEXT://:9092,CONTROLLER://:9093 \
    -e KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://127.0.0.1:9092 \
    -e ALLOW_PLAINTEXT_LISTENER=yes \
    soldevelo/kafka:latest
```

## 使用方式

### 基础：发布/订阅

```go
b := kafka.NewBroker(
    broker.WithAddress("127.0.0.1:9092"),
    broker.WithCodec("json"),
)
b.Init()
b.Connect()
defer b.Disconnect()

// 发布
b.Publish(ctx, "test-topic", broker.NewMessage(msg))

// 订阅（自动消费组）
sub, _ := b.Subscribe("test-topic", handler, binder,
    broker.WithSubscribeQueueName("my-group"),
)
```

### 高级：SASL 认证

```go
b := kafka.NewBroker(
    broker.WithAddress("127.0.0.1:9092"),
    kafka.WithScramMechanism(kafka.ScramAlgorithmSHA256, "user", "pass"),
    broker.WithCodec("json"),
)
```

### 高级：批量消费

```go
sub, _ := b.Subscribe("test-topic", handler, binder,
    broker.WithSubscribeQueueName("my-group"),
    kafka.WithSubscribeBatchSize(100),
    kafka.WithSubscribeBatchInterval(time.Second),
)
```

### 高级：自定义负载均衡器

```go
b.Publish(ctx, "test-topic", broker.NewMessage(msg),
    kafka.WithMurmur2Balancer(true),
)
```

## 配置选项

### Broker 选项

| 选项 | 说明 |
|------|------|
| `kafka.WithReaderConfig(cfg)` | 原生 Reader 配置 |
| `kafka.WithWriterConfig(cfg)` | 原生 Writer 配置 |
| `kafka.WithEnableOneTopicOneWriter(enable)` | 每个 Topic 使用独立 Writer |
| `kafka.WithPlainMechanism(user, pass)` | PLAIN SASL 认证 |
| `kafka.WithScramMechanism(algo, user, pass)` | SCRAM SHA256/SHA512 认证 |
| `kafka.WithBatchSize(n)` | 发送批次大小（默认 100） |
| `kafka.WithBatchTimeout(d)` | linger.ms（默认 10ms） |
| `kafka.WithBatchBytes(n)` | 批次最大字节数（默认 1048576） |
| `kafka.WithAsync(enable)` | 异步发送（默认 true） |
| `kafka.WithReadTimeout(d)` | 读取超时（默认 10s） |
| `kafka.WithWriteTimeout(d)` | 写入超时（默认 10s） |
| `kafka.WithEnableLogger(enable)` | 启用框架 info 日志 |
| `kafka.WithEnableErrorLogger(enable)` | 启用框架 error 日志 |
| `kafka.WithAllowPublishAutoTopicCreation(enable)` | 允许自动创建 Topic |
| `kafka.WithCompletion(fn)` | 消息发布完成回调 |

### Publish 选项

| 选项 | 说明 |
|------|------|
| `kafka.WithLeastBytesBalancer()` | LeastBytes 负载均衡 |
| `kafka.WithRoundRobinBalancer()` | RoundRobin 负载均衡（默认） |
| `kafka.WithHashBalancer(hasher)` | Hash 负载均衡 |
| `kafka.WithCrc32Balancer(consistent)` | CRC32 负载均衡 |
| `kafka.WithMurmur2Balancer(consistent)` | Murmur2 负载均衡 |

### Subscribe 选项

| 选项 | 说明 |
|------|------|
| `kafka.WithSubscribeAutoCreateTopic(topic, parts, replicas)` | 订阅时自动创建 Topic |
| `kafka.WithQueueCapacity(n)` | 内部消息队列容量 |
| `kafka.WithMinBytes(n)` | fetch.min.bytes |
| `kafka.WithMaxBytes(n)` | fetch.max.bytes |
| `kafka.WithMaxWait(d)` | fetch.max.wait.ms |
| `kafka.WithCommitInterval(d)` | 提交间隔 |
| `kafka.WithSessionTimeout(d)` | 会话超时 |
| `kafka.WithRebalanceTimeout(d)` | 重平衡超时 |
| `kafka.WithStartOffset(offset)` | 起始偏移量 |
| `kafka.WithSubscribeBatchSize(n)` | 批量消费大小 |
| `kafka.WithSubscribeBatchInterval(d)` | 批量消费间隔 |
| `kafka.WithRetries(n)` | 消费重试次数 |

## 管理工具

- [Offset Explorer](https://www.kafkatool.com/download.html)

## 参考资料

* [使用kafka-go导致的消费延时问题](https://loesspie.com/2020/12/28/kafka-golang-segmentio-kafka-go-slow-cousume/)
* [kafka-go 读取kafka消息丢失数据的问题定位和解决](https://cloud.tencent.com/developer/article/1809467)
* [Go社区主流Kafka客户端简要对比](https://tonybai.com/2022/03/28/the-comparison-of-the-go-community-leading-kakfa-clients/)
* [kafka go Writer 写入消息过慢的原因分析](http://timd.cn/kafka-go-writer/)
