# Go-Wind 插件教程 · 第 5 章：编解码（Encoding）

> 本篇是 [Go-Wind 插件系列教程](./README.md)的组成部分。本章是 [`encoding/README.md`](../../encoding/README.md) 的导读篇——该模块 README 本身已是完整文档，这里给出全景与导航。

## 1. 这一层解决什么问题

序列化格式是横切关注点：同一份业务对象，HTTP API 要 JSON，内部 gRPC 要 Protobuf，消息管道可能是 MsgPack。编解码层把「格式」从业务代码里剥离——所有传输层（HTTP / gRPC / TCP / WebSocket / SSE 等）统一通过 `WithCodec(name)` 指定格式，业务对象零感知。

## 2. 接口架构

| 接口/函数 | 方法 | 说明 |
|------|------|------|
| `Codec` | `Marshal(v) ([]byte, error)` | 序列化 |
| `Codec` | `Unmarshal(data, v) error` | 反序列化 |
| `Codec` | `Name() string` | 编解码器名称（`json` / `proto` / `yaml`……） |
| 包函数 | `RegisterCodec(c)` / `GetCodec(name)` | 全局注册表（`init()` 自注册、按名取用，名称大小写无关） |

设计上，各编解码器通过 `init()` 自注册，业务侧以空导入（`_ "github.com/tx7do/go-wind-plugins/encoding/json"`）引入所需格式——未引入的格式不进二进制。自定义格式实现三方法后在 `init()` 里 `RegisterCodec` 即可接入，与内置格式完全同权。

## 3. 传输层接入形态

各传输服务器的构造选项接受编解码器名，一行切换：

```go
srv := tcp.NewServer(
    tcp.WithAddress(":9000"),
    tcp.WithCodec("json"),
)
```

HTTP、SSE 等服务器同理（`WithCodec`）。HTTP 侧另有内容协商中间件（`transport/http/middleware/codec`）按请求头协商格式，见[第 8 章](./08-transport-middleware.md)。

## 4. 格式矩阵与选型

下表复述自[模块 README](../../encoding/README.md)的「支持的编解码格式」一节，名称、包路径与说明均为原文：

#### 文本格式

| 名称 | 包路径 | 说明 |
|------|--------|------|
| `json` | `encoding/json` | JSON（标准库），通用性最强 |
| `xml` | `encoding/xml` | XML（标准库），SOAP / 传统系统 |
| `yaml` | `encoding/yaml` | YAML（gopkg.in/yaml.v3），配置文件 |
| `toml` | `encoding/toml` | TOML（BurntSushi/toml），配置文件 |

#### 二进制格式

| 名称 | 包路径 | 说明 |
|------|--------|------|
| `proto` | `encoding/proto` | Protocol Buffers，高性能 RPC |
| `msgpack` | `encoding/msgpack` | MessagePack（vmihailenco/msgpack/v5），紧凑二进制 |
| `bson` | `encoding/bson` | BSON（mongo-driver），MongoDB 原生格式 |
| `cbor` | `encoding/cbor` | CBOR（fxamacker/cbor/v2），RFC 8949，WebAuthn / COSE |
| `gob` | `encoding/gob` | Go Gob（标准库），Go-to-Go 内部通信 |
| `thrift` | `encoding/thrift` | Apache Thrift 二进制协议，需 TStruct 生成代码 |

#### Schema 驱动格式

| 名称 | 包路径 | 说明 |
|------|--------|------|
| `avro` | `encoding/avro` | Apache Avro（linkedin/goavro/v2），大数据 / Kafka |
| `flatbuffers` | `encoding/flatbuffers` | Google FlatBuffers，零拷贝序列化 |

选型的常识约束：跨语言 API 用 `json` 或 `proto`；Go 内部通信 `gob` 最省事但锁死 Go；`msgpack` / `cbor` 体积小但要求消费端同样理解格式；`avro` / `flatbuffers` 面向大数据与游戏/实时系统，需要 schema 或生成代码配合。完整对比表（可读性/性能/体积/跨语言）见[模块 README](../../encoding/README.md)。

## 5. 特殊格式的硬约束

三种格式有编译期约束，传入参数类型不对会直接报错：

- **`proto`**：`Marshal` / `Unmarshal` 的参数必须实现 `proto.Message`（protoc 生成结构体）。
- **`thrift`**：参数必须实现 `thrift.TStruct`（thrift 编译器生成）。
- **`flatbuffers`**：需要 `flatc` 生成的 `FlatBufferMarshaler` / `FlatBuffer` 实现，零拷贝读写。
- **`avro`**：默认 `"avro"` 编解码器使用空 schema 只支持原始类型；复杂记录需 `avro.NewCodec(schema)` 带 schema 构造独立实例。

## 6. 设计原则与红线

模块 README 确立的红线值得复述：**禁止在业务代码手写编解码逻辑**——一律走 `Codec` 接口；格式选择在组装服务器时声明，不在 handler 里分支；只引入用到的格式控制二进制体积。

## 7. 深入阅读

- [`encoding/README.md`](../../encoding/README.md) —— 完整 API 参考、格式对比表、自定义编解码器
- [根 README · 编解码矩阵](../../README.md#编解码encoding)
- [第 13 章 · CRUD 契约](./13-crud-contracts.md) —— proto 契约在数据访问层的应用
