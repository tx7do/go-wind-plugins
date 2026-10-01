# Go-Wind 插件教程 · 第 22 章：终章——全家族装配

> 本篇是 [Go-Wind 插件系列教程](./README.md)的最后一章。前面 21 章逐家族讲清了「选哪个、怎么接」；本章把它们装配成一个完整服务，并给出全项目的覆盖对照表。

## 1. 装配原则：乐高式组合

本生态的设计原则是**接口优先、实现可选**：

1. **核心框架定义接口**——`go-wind` 框架定义 `Reader`、`Registrar`、`Logger`、`Server` 等标准接口；
2. **插件实现接口**——每个插件模块只实现对应接口（如 etcd 配置中心实现 `config.Reader` + `config.ValueWatcher`）；
3. **应用层注入**——业务代码通过接口引用插件，编译时选择具体实现，**切换引擎只需改一行 import**。

配套的工程形态是**独立版本管理**：每个子模块拥有独立 `go.mod`、独立发布（接口包如 `github.com/tx7do/go-wind-plugins/config`，实现包如 `github.com/tx7do/go-wind-plugins/config/etcd`）——按需引入、单独升级，未引入的模块不进二进制。

## 2. 安装模式

```bash
# 按需引入，例如 etcd 配置中心 + nacos 注册中心 + zap 日志
go get github.com/tx7do/go-wind-plugins/config/etcd
go get github.com/tx7do/go-wind-plugins/registry/nacos
go get github.com/tx7do/go-wind-plugins/log/zap
```

每个家族的可用模块路径清单见[根 README 插件矩阵](../../README.md)与本系列各章第 3 节的矩阵表。开发本仓自身则用 `go.work` 把全部模块聚合为单仓体验（见仓库根）。

## 3. 一次完整装配

下面按装配顺序走一遍，每步的完整选项与语义见对应章节。示例刻意保持最小——生产形态的差异点在各步末尾标注。

### 3.1 传输层与驱动

```go
import (
    httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
    "github.com/tx7do/go-wind-plugins/transport/http/driver/gin"
)

srv := httpPlugin.NewServer(":8080",
    httpPlugin.WithDriver(gin.NewDriver()),
    httpPlugin.WithMiddleware(/* 见第 8 章中间件套件 */),
)
srv.GET("/", handler)
```

驱动选择（std / gin / chi / fiber）的实测对比见[第 20 章](./20-http-utilities.md)基准报告——结论是按功能面选，不按跑分选。服务器生命周期由框架 `WithServer` 统一管理（[第 8 章](./08-transport-middleware.md)）。

### 3.2 API 文档挂载

```go
swagger.Register(srv,
    swagger.WithTitle("Petstore"),
    swagger.WithRemoteFileURL("https://petstore3.swagger.io/api/v3/openapi.json"),
    swagger.WithBasePath("/docs/"),
)
```

三种文档源与 ReDoc 备选见[第 20 章](./20-http-utilities.md)。生产环境给文档端点配认证。

### 3.3 日志

```go
import (
    zap "go.uber.org/zap"
    zaplog "github.com/tx7do/go-wind-plugins/log/zap"
    wind "github.com/tx7do/go-wind"
)

zlog, _ := zap.NewProduction()
wind.New(wind.WithLogger(zaplog.NewZapLogger(zlog)))
```

引擎选型与已知问题（`log/aliyun` 构建失败）见[第 3 章](./03-log.md)。

### 3.4 追踪与指标

```go
tp, err := otlp.New(
    otlp.WithEndpoint("localhost:4317"),
    otlp.WithServiceName("my-service"),
    otlp.WithServiceVersion("v1.0.0"),
    otlp.WithSampleRatio(1.0),
    otlp.WithInsecure(true),
)
defer tp.Shutdown(ctx)   // 退出前刷新未导出 span
```

本地用 Jaeger all-in-one 容器起步（`docker run -p 4317:4317 -p 16686:16686 jaegertracing/jaeger`）。指标侧 `prometheus.NewWithDefaultRegistry(prometheus.WithNamespace("myapp"))` 挂 `/metrics` 抓取端点。两者语义、采样与命名规范见[第 6 章](./06-tracer.md)与[第 4 章](./04-metrics.md)——框架内传输层与消息代理服务器的 `WithMetrics` 埋点会自动打点请求侧指标。

### 3.5 注册发现

```go
r := nacos.New(namingClient)
instance := &wind.Instance{
    Name: "my-service", Version: "v1.0.0",
    Endpoints: []string{"grpc://127.0.0.1:8080"},
}
_ = r.Register(context.Background(), instance) // 必须发生在服务器 Start 之前
```

注销放 `wind.WithBeforeStop(...)`——先从注册中心消失再关服务器。完整生命周期与引擎选项见[第 2 章](./02-registry.md)。

### 3.6 配置

```go
cfg, _ := etcd.New(etcdClient)
data, _ := cfg.Load(context.Background(), "/myapp/config")
ch, _ := cfg.WatchValue(context.Background(), "/myapp/config") // 推值监听热更新
```

配置源选型（本地 dev 用 file/env，K8s 用 ConfigMap，密钥走 vault）见[第 1 章](./01-config.md)。

### 3.7 消息代理

```go
b := kafkaBroker.NewBroker(
    broker.WithAddress("localhost:9092"),
    broker.WithCodec("json"),
)
b.Init(); b.Connect(); defer b.Disconnect()
b.Publish(ctx, "sensor.temperature",
    broker.NewMessage(payload, broker.WithPublishHeaders(map[string]string{"version": "1.0"})))
```

消费侧有库形态与传输层服务器形态两条路（[第 7 章](./07-broker.md)）；消费 handler 必须幂等。编解码器经 `broker.WithCodec` 指定（[第 5 章](./05-encoding.md)）。

### 3.8 AI 集成

```go
cfg := &langchaingo.Config{
    Type: langchaingo.ModelTypeCloud, ModelName: "gpt-4o",
    Cloud: &langchaingo.CloudConfig{ApiKey: "sk-xxx", BaseUrl: "https://api.openai.com/v1"},
    TimeoutSeconds: 60,
}
llm, _ := langchaingo.NewModel(cfg)
resp, _ := llm.Call(ctx, "...")
```

各模块自有的 `Config` 同构承载构造入参（字段一致、定义独立），密钥经配置层注入。见[第 18 章](./18-ai.md)。

### 3.9 数据访问层

有持久化需求时，按[第 12 章](./12-crud-overview.md)的选型表选引擎、[第 13 章](./13-crud-contracts.md)的契约定义查询、按需叠加[第 15 章](./15-crud-integrations.md)的缓存/审计/数据权限。这一层的接线形态与上面各家族不同（Repository 泛型而非 Server），按第 12–15 章走。

### 3.10 弹性、安全、健康

出站调用包[第 9 章](./09-resilience.md)三件套；入口挂 [第 10 章](./10-security.md)认证授权中间件与[第 8 章](./08-transport-middleware.md)的治理中间件链；探活端点按[第 19 章](./19-tools.md)接真实依赖。工作流、对象存储按[第 16 章](./16-workflow-transaction.md)、[第 17 章](./17-oss.md)。

## 4. 生命周期总图

框架 `wind.New` 的选项面（`WithServer` / `WithLogger` / `WithBeforeStop` / `WithAfterStop` / `WithStopTimeout`）决定了装配后的时序：

| 阶段 | 动作 | 涉及家族 |
|------|------|----------|
| 启动前 | 注册实例到注册中心 | registry（ch02） |
| 启动 | 各 Server `Start`、broker `Connect`、配置 `WatchValue` 生效 | transport / broker / config |
| 运行 | 指标埋点、追踪 span、日志、限流熔断全程生效 | metrics / tracer / log / resilience |
| 收到信号 | `WithBeforeStop` 钩子：**先**注销实例、停止接流 | registry（ch02） |
| 优雅停机 | 各 Server `Stop`、`tp.Shutdown`（刷新 span）、metrics `Close`、broker `Disconnect`、配置源 `Close` | 全部 |

任何资源如果只在 `defer` 里清理而没进 `WithBeforeStop`，就会在停止窗口里继续对外服务——这是装配期最常见的一个坑。

## 5. 生产检查单

- [ ] TLS：对外端口全 TLS，追踪/指标端点走网关
- [ ] 文档端点（swagger/redoc）、pprof、`/metrics` 都在认证面内或内网隔离
- [ ] 密钥零硬编码：vault / K8s Secret + [第 1 章](./01-config.md)注入
- [ ] 消费 handler 幂等；毒消息有死信出口（[第 7 章](./07-broker.md)）
- [ ] 缓存键含租户/归属维度（[第 11 章](./11-cache.md)、[第 15 章](./15-crud-integrations.md)）
- [ ] 数据范围过滤与审计层在数据服务上启用（[第 15 章](./15-crud-integrations.md)）
- [ ] 追踪采样率从容起步、日志带 trace-id（[第 6 章](./06-tracer.md)、[第 3 章](./03-log.md)）
- [ ] 健康探针探真实依赖（[第 19 章](./19-tools.md)）
- [ ] 出站调用全部包弹性三件套（[第 9 章](./09-resilience.md)）

## 6. 全项目覆盖对照表

| 家族 | 章节 | 引擎/模块数 |
|------|------|-------------|
| 配置中心 | [01](./01-config.md) | 13 引擎 + 接口包 |
| 注册发现 | [02](./02-registry.md) | 8 |
| 日志 | [03](./03-log.md) | 13 |
| 指标监控 | [04](./04-metrics.md) | 3 |
| 编解码 | [05](./05-encoding.md) | 12 |
| 追踪 | [06](./06-tracer.md) | 1（OTLP） |
| 消息代理（库形态） | [07](./07-broker.md) | 12 |
| 传输层与中间件总览 | [08](./08-transport-middleware.md) | 协议矩阵 22 行 + 两侧中间件模块 17 / 16 个 |
| HTTP 工具族 | [20](./20-http-utilities.md) | 4（swagger / redoc / binding / benchmark） |
| 协议模块文档地图（含 12 个消息代理承载型） | [21](./21-protocol-modules.md) | 本章导航的全部协议模块（协议矩阵 22 + 承载型 12，与第 8 章矩阵部分重叠） |
| 弹性治理 | [09](./09-resilience.md) | 熔断 4 + 限流 3 + 重试 |
| 安全 | [10](./10-security.md) | 认证 9 + 授权 8 + 加密 |
| 通用缓存 | [11](./11-cache.md) | 2 |
| 数据访问层（CRUD） | [12](./12-crud-overview.md)–[15](./15-crud-integrations.md) | 14（8 引擎 + 5 设施 + cassandra 开发中） |
| 工作流与事务 | [16](./16-workflow-transaction.md) | 工作流 4 引擎 + 事务 5 模块（dtm / outbox / saga / tcc / xa），各含接口根模块 |
| 对象存储 | [17](./17-oss.md) | 2 |
| AI 集成 | [18](./18-ai.md) | 3 |
| 工具模块 | [19](./19-tools.md) | errors / health / pprof / retry / testing |

本表即「系列覆盖整个项目」的对照基准；后续新增模块时应随矩阵同步补章。
