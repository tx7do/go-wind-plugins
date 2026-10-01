# Go-Wind 插件教程 · 第 2 章：服务注册发现（Registry）

> 本篇是 [Go-Wind 插件系列教程](./README.md)的组成部分。注册发现插件实现根模块 [`github.com/tx7do/go-wind-plugins/registry`](../../registry/) 定义的 `Registrar` / `Discovery` 接口；实例类型 `Instance` 由 [go-wind 框架](https://github.com/tx7do/go-wind)提供。

## 1. 这一层解决什么问题

服务A要调用服务B，B 的 IP:端口 在滚动更新、扩缩容、故障漂移中不断变化。注册发现层把「B 当前有哪些健康实例」这件事交给注册中心维护：提供方上线时注册、下线时注销，消费方按服务名查询并监听变更。业务代码只持有一个服务名，不持有任何地址。

## 2. 接口架构

根模块 `registry` 的接口面：

| 接口 | 方法 | 说明 |
|------|------|------|
| `Registrar` | `Register(ctx, *Instance)` / `Deregister(ctx, *Instance)` | 服务实例的注册与注销 |
| `Discovery` | `GetService(ctx, name)` / `Watch(ctx, name)` | 按服务名查询实例、建立变更监听 |
| `Watcher` | `Next(ctx) ([]*Instance, error)` / `Stop()` | 实例列表变更流：有变化时 `Next` 返回新快照 |

`Instance` 结构（定义于 go-wind 框架 `instance.go`）携带 `ID` / `Name` / `Version` / `Endpoints` / `Metadata` 字段，描述一个可寻址的服务实例。不同注册中心支持的元数据粒度不同，`Metadata` 的可用键以各引擎文档为准。

## 3. 插件矩阵

| 插件 | 模块路径 | 引擎 |
|------|---------|------|
| Consul | `github.com/tx7do/go-wind-plugins/registry/consul` | HashiCorp Consul |
| Etcd | `github.com/tx7do/go-wind-plugins/registry/etcd` | CoreOS etcd |
| Eureka | `github.com/tx7do/go-wind-plugins/registry/eureka` | Netflix Eureka |
| Kubernetes | `github.com/tx7do/go-wind-plugins/registry/kubernetes` | K8s Endpoints |
| Nacos | `github.com/tx7do/go-wind-plugins/registry/nacos` | 阿里云 Nacos |
| Polaris | `github.com/tx7do/go-wind-plugins/registry/polaris` | 腾讯云 Polaris |
| ServiceComb | `github.com/tx7do/go-wind-plugins/registry/servicecomb` | Apache ServiceComb |
| Zookeeper | `github.com/tx7do/go-wind-plugins/registry/zookeeper` | Apache ZooKeeper |

## 4. 接入示例：etcd 注册

以 `registry/etcd` 为例（完整版本见 [`registry/etcd/example_test.go`](../../registry/etcd/example_test.go)）。生命周期约定：**注册发生在服务器开始接受流量之前**（消费者能立即发现新实例），**注销放在应用的 `BeforeStop` 钩子里**（先从注册中心消失，再关闭服务器，消费者不会再把流量打到正在关闭的实例上）：

```go
client, err := clientv3.New(clientv3.Config{
    Endpoints:   []string{"localhost:2379"},
    DialTimeout: 5 * time.Second,
})
if err != nil {
    return
}
defer client.Close()

reg := etcd.New(client,
    etcd.Namespace("/microservices"), // 键空间前缀
    etcd.RegisterTTL(15*time.Second),  // 租约 TTL，实例异常退出后自动过期
)

instance := &wind.Instance{
    ID:        "registry-demo-001",
    Name:      "demo-service",
    Version:   "1.0.0",
    Endpoints: []string{"http://localhost:8080"},
    Metadata:  map[string]string{"protocol": "http"},
}

_ = reg.Register(context.Background(), instance)     // 服务器 Start 之前
_ = reg.Deregister(context.Background(), instance)   // 对应 WithBeforeStop 钩子
```

etcd 引擎的全部选项即上面出现的 `Namespace` 与 `RegisterTTL`。其他引擎的选项面不同——例如 consul 引擎提供 `WithHealthCheck` / `WithHealthCheckInterval` / `WithHeartbeat` / `WithDatacenter` / `WithServiceResolver` / `WithTimeout`，用于控制健康检查与数据中心路由——接入前先读对应模块源码的 options 文件确认可用项。

消费侧（`GetService` / `Watch` + `Next`）拿到的是 `[]*Instance` 快照，把它喂给负载均衡器使用；变更流每次返回全量快照，消费方做幂等替换即可，不要对快照做增量 diff 假设。

## 5. 选型与运维要点

- **K8s 部署优先 `kubernetes` 引擎**：直接消费平台 Endpoints，不引入额外设施，且与平台的探针、滚动更新语义天然对齐。
- **已有注册中心**的团队选对应插件，避免双设施。
- **租约 TTL 不是健康检查**：TTL 只保证「进程死了记录会过期」，实例是否真的能服务流量要靠健康检查（[第 19 章 health 模块](./19-tools.md)提供探活端点）配合注册中心的健康检查机制。
- 注册的 `Metadata` 会广播给所有消费者，不要放敏感信息。

## 6. 深入阅读

- [`registry/etcd/example_test.go`](../../registry/etcd/example_test.go) —— 注册/注销生命周期
- [根 README · 服务注册发现矩阵](../../README.md#服务注册发现registry)
- 各引擎源码目录：[`registry/`](../../registry/)
