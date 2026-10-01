# Go-Wind 插件库系列教程

本系列按插件矩阵的领域划分，系统性地讲解 `go-wind-plugins` 各插件家族的定位、接口架构、引擎矩阵与接入方式。目标是让读者在「选哪个引擎、怎么接、接的时候注意什么」三个问题上快速得到可靠答案。

## 文档体系

本仓库的文档分三层，各司其职：

| 层 | 位置 | 内容 |
|----|------|------|
| 系列教程 | `docs/tutorials/`（本目录） | 领域级导读：每个插件家族一章，覆盖接口、矩阵、典型接入、选型 |
| 深度实战教程 | `docs/http-server-tutorial.md` 等 | 单一主题的完整实战（千行级，含可运行示例） |
| 模块 README | 各模块目录内 | 引擎级深度文档：具体引擎的全部配置项、API、部署与示例 |
| 总索引 | [仓库根 README](../../README.md) | 插件矩阵总表、核心接口定义 |

**约定**：本系列所有示例代码使用本仓发布路径（`github.com/tx7do/go-wind-plugins/...`）；每个子模块是独立 Go module，按需 `go get` 引入；仓库内部开发用 `go.work` 聚合（当前 go 指令 1.26.4）。接入 go-wind 框架时，应用级选项以框架 `app` 包为准（当前提供 `WithLogger` / `WithServer` / `WithBeforeStop` / `WithAfterStop` 等），其余插件家族直接以库形式在业务代码中使用。

## 目录

### 第一部分 · 框架核心域

| 章 | 主题 | 文件 |
|----|------|------|
| 1 | 配置中心（Config） | [01-config.md](./01-config.md) |
| 2 | 服务注册发现（Registry） | [02-registry.md](./02-registry.md) |
| 3 | 日志（Log） | [03-log.md](./03-log.md) |
| 4 | 指标监控（Metrics） | [04-metrics.md](./04-metrics.md) |
| 5 | 编解码（Encoding） | [05-encoding.md](./05-encoding.md) |
| 6 | 分布式追踪（Tracer） | [06-tracer.md](./06-tracer.md) |
| 7 | 消息代理（Broker） | [07-broker.md](./07-broker.md) |
| 8 | 传输层与中间件（Transport & Middleware） | [08-transport-middleware.md](./08-transport-middleware.md) |

### 第二部分 · 弹性与安全

| 章 | 主题 | 文件 |
|----|------|------|
| 9 | 熔断、限流与重试（弹性治理） | [09-resilience.md](./09-resilience.md) |
| 10 | 认证、授权与加密（Security） | [10-security.md](./10-security.md) |
| 11 | 通用缓存（Cache） | [11-cache.md](./11-cache.md) |

### 第三部分 · 数据访问层（CRUD 家族）

| 章 | 主题 | 文件 |
|----|------|------|
| 12 | CRUD 总览与家族架构 | [12-crud-overview.md](./12-crud-overview.md) |
| 13 | 分页、过滤与排序契约 | [13-crud-contracts.md](./13-crud-contracts.md) |
| 14 | 引擎适配与选型 | [14-crud-engines.md](./14-crud-engines.md) |
| 15 | 缓存、审计与数据权限集成 | [15-crud-integrations.md](./15-crud-integrations.md) |

### 第四部分 · 外部资源与工具

| 章 | 主题 | 文件 |
|----|------|------|
| 16 | 工作流与分布式事务 | [16-workflow-transaction.md](./16-workflow-transaction.md) |
| 17 | 对象存储（OSS） | [17-oss.md](./17-oss.md) |
| 18 | AI 大模型集成 | [18-ai.md](./18-ai.md) |
| 19 | 工具模块（errors / health / pprof / testing） | [19-tools.md](./19-tools.md) |

### 第五部分 · 补全与装配

| 章 | 主题 | 文件 |
|----|------|------|
| 20 | HTTP 工具族（swagger / redoc / binding / benchmark） | [20-http-utilities.md](./20-http-utilities.md) |
| 21 | 协议模块文档地图（含消息代理承载型归位） | [21-protocol-modules.md](./21-protocol-modules.md) |
| 22 | 终章——全家族装配与覆盖对照 | [22-composition.md](./22-composition.md) |

### 配套深度实战教程

| 教程 | 覆盖内容 |
|------|----------|
| [Go-Wind HTTP 服务器从入门到精通](../http-server-tutorial.md) | HTTP 服务器、驱动系统（std/gin/chi/fiber）、中间件机制、自定义中间件、Chain、TLS |
| [Go-Wind gRPC 服务器从入门到精通](../grpc-server-tutorial.md) | gRPC 服务器与 gRPC 中间件套件 |
| [Go-Wind GraphQL 服务器从入门到精通](../graphql-server-tutorial.md) | GraphQL 服务器搭建 |

## 阅读路径建议

- **初次接触本生态**：先读第 8 章了解传输层全貌，再配合 HTTP 实战教程跑通第一个服务，然后按需跳读其余各章。
- **搭建数据服务**：第 12 → 13 → 14 → 15 章顺序阅读，配合目标引擎的模块 README。
- **只关心某个引擎**：直接从上表进入对应章的「深入阅读」小节，跳转到模块 README。

## 状态说明

- 全部模块按 `v0.0.1` 标签发布（见 `tag.sh`），模块间互引固定 `v0.0.1`。
- `crud/cassandra` 为开发中状态，尚未可用。
- `log/aliyun` 存在 SDK 工具链不兼容的构建问题（详见第 3 章）。
