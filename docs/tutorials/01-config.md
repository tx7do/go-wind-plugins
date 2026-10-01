# Go-Wind 插件教程 · 第 1 章：配置中心（Config）

> 本篇是 [Go-Wind 插件系列教程](./README.md)的组成部分。配置插件实现的是根模块 [`github.com/tx7do/go-wind-plugins/config`](../../config/) 定义的 Source 族接口。

## 1. 这一层解决什么问题

同一个服务二进制要在开发、测试、生产多个环境运行，数据库地址、特性开关、限流阈值等参数各不相同。把它们从代码里挪出去，交给外部存储统一管理，就是配置中心层的职责。本仓把「配置从哪来」抽象成 Source 接口族，引擎插件只负责对接某一种存储；业务代码面对的是统一的加载与监听 API，切换配置后端不改业务代码。

## 2. 接口架构

根模块 `config` 定义了读写配置的最小接口面：

| 接口 | 方法 | 语义 |
|------|------|------|
| `Reader` | `Load(ctx, key) ([]byte, error)` | 按 key 一次性读取配置内容 |
| `Watcher` | `Watch(ctx, key) (<-chan struct{}, error)` | 信号模式：key 对应内容变更时发信号，值需另行读取 |
| `ValueWatcher` | `WatchValue(ctx, key) (<-chan []byte, error)` | 推值模式：直接把新值推送到 channel |
| `Closer` | `Close() error` | 释放底层连接 |
| `Decoder` | `Decode(data, out) error` | 把原始字节解码进目标结构（配合[第 5 章编解码](./05-encoding.md)的 Codec 使用） |

`ReadCloser` / `ReadWatcher` 等组合接口由上述接口组合而成。引擎插件（如 `config/file`）返回的实现即满足其中若干接口；调用方通过类型断言或直接按已知引擎能力编程。

两种监听模式的取舍：信号模式省流量但需要回读；推值模式实时性好但要求配置体积可控。生产环境推荐推值模式配合小体积配置项，大对象配置（证书、策略文件）用信号模式 + 回读。

## 3. 插件矩阵

| 插件 | 模块路径 | 引擎 |
|------|---------|------|
| Apollo | `github.com/tx7do/go-wind-plugins/config/apollo` | 携程 Apollo |
| Consul | `github.com/tx7do/go-wind-plugins/config/consul` | HashiCorp Consul KV |
| Etcd | `github.com/tx7do/go-wind-plugins/config/etcd` | CoreOS etcd |
| Env | `github.com/tx7do/go-wind-plugins/config/env` | 环境变量 |
| File | `github.com/tx7do/go-wind-plugins/config/file` | 本地文件 |
| FS | `github.com/tx7do/go-wind-plugins/config/fs` | `fs.FS`（嵌入文件系统） |
| HTTP | `github.com/tx7do/go-wind-plugins/config/http` | HTTP 远程拉取 |
| Kubernetes | `github.com/tx7do/go-wind-plugins/config/kubernetes` | K8s ConfigMap / Secret |
| Nacos | `github.com/tx7do/go-wind-plugins/config/nacos` | 阿里云 Nacos |
| OSS | `github.com/tx7do/go-wind-plugins/config/oss` | 对象存储 |
| Polaris | `github.com/tx7do/go-wind-plugins/config/polaris` | 腾讯云 Polaris |
| Redis | `github.com/tx7do/go-wind-plugins/config/redis` | Redis KV |
| Vault | `github.com/tx7do/go-wind-plugins/config/vault` | HashiCorp Vault |
| Zookeeper | `github.com/tx7do/go-wind-plugins/config/zookeeper` | Apache ZooKeeper |

## 4. 接入示例：本地文件源

以 `config/file` 为例（完整可运行版本见 [`config/file/example_test.go`](../../config/file/example_test.go)）：

```go
src, err := file.New(
    file.WithPath("/etc/myapp/config.json"),
    file.WithWatch(true),
)
if err != nil {
    return
}
defer src.Close()

// 一次性读取
raw, err := src.Load(context.Background(), "")

// 推值监听：文件被修改时，新内容从 channel 推出
ch, err := src.WatchValue(context.Background(), "")
```

`config/file` 全部选项：`WithPath`（文件路径）、`WithWatch`（是否启用监听）、`WithContext`。`config/env` 无选项，直接从进程环境变量读取，常用于容器化部署里注入少量部署期参数。

典型组合模式：本地开发用 `file` + `env`，不依赖任何外部设施；生产环境按平台选择集中式配置存储。多源聚合（例如默认值来自嵌入 FS、环境特定值来自 K8s ConfigMap）由业务侧自行合并——Source 接口刻意保持最小面，聚合策略不属于本层职责。

## 5. 选型建议

| 场景 | 建议 |
|------|------|
| 本地开发 / 单元测试 | `file` / `env` / `fs`（零外部依赖） |
| Kubernetes 部署 | `kubernetes`（ConfigMap / Secret 是平台原生方案） |
| 已有 Consul / etcd / Nacos / ZooKeeper 集群 | 对应插件，避免再引入新设施 |
| 密钥、证书类敏感配置 | `vault`（动态密钥与审计能力） |
| 配置内容本身就是对象（大文件、制品） | `oss` 配合[第 17 章对象存储](./17-oss.md) |

通用原则：配置里只放「随环境变化的参数」，不放业务数据；任何写进配置的凭据都要规划轮转。

## 6. 深入阅读

- [`config/file/example_test.go`](../../config/file/example_test.go) —— 文件源完整用法
- [根 README · 配置中心矩阵](../../README.md#配置中心config)
- 各引擎源码目录：[`config/`](../../config/)
