# Go-Wind 插件教程 · 第 14 章：CRUD 引擎适配与选型

> 本篇是 [Go-Wind 插件系列教程](./README.md)的组成部分。每个引擎模块的 README 都是千行级完整文档（含 Docker 部署与全部配置项）——本章是它们的选型导航与文档地图。

## 0. 引擎模块的共同骨架

各引擎模块内部结构同构，理解一个即理解全部：

| 子包 | 职责 |
|------|------|
| `client.go` / `options.go` | 引擎客户端构造与配置选项 |
| `repository.go` | 泛型 `Repository[TEntity, TDTO]` 实现，统一 CRUD 语义 |
| `field/` | [字段掩码](./13-crud-contracts.md#5-排序与字段掩码)到引擎投影的翻译 |
| `filter/` | FilterExpr 中间结构到引擎查询谓词的翻译（含安全测试） |
| `pagination/` | 分页器方言（page / offset / token 三种模式的引擎落地） |
| `sorting/` | 排序结构到引擎 ORDER BY 的翻译 |
| `mixin/` | 跨切字段注入（`tenant_id` 多租户、审计字段等） |

写入路径的形态依引擎而定（见各模块文档）；`doris` 与 `clickhouse` 额外提供批量写入（`batch.go`），`doris` 支持 Stream Load 高速导入。

## 1. GORM —— 通用关系型 ORM

- **定位**：MySQL / PostgreSQL / SQLite / SQL Server 等主流关系库的通用业务存储。
- **文档地图**（[`crud/gorm/README.md`](../../crud/gorm/README.md)）：快速开始（Entity/DTO 定义、创建 Repository、基本 CRUD）→ 高级功能（分页查询、带过滤与排序的分页、FieldMask 字段选择、缓存功能、计数查询、存在性检查）→ GORM Client（创建客户端、**读写分离**、**自动迁移**、**连接池配置**）→ API 参考（Repository 方法面、Client 配置选项）→ 最佳实践（事务、缓存、错误处理、性能优化）→ 测试与示例项目。
- **特殊能力**：Upsert（INSERT ON CONFLICT 原生支持）；缓存功能与 [`crud/cache`](../../crud/cache/README.md) 集成（[第 15 章](./15-crud-integrations.md)）。

## 2. Ent —— 代码生成型关系型 ORM

- **定位**：以 schema 为源头的编译期类型安全 ORM，适合 schema 演进受控、模型关系复杂的业务库。
- **文档地图**（[`crud/entgo/README.md`](../../crud/entgo/README.md)）：快速开始（定义 Ent Schema → 生成 Ent 代码 → 定义 Protobuf DTO → 创建 Repository → 基本 CRUD）→ 高级功能（分页查询、通用分页请求）。
- **特殊能力**：模块内 `mixin/` 提供审计字段（`audit.go`）、软删除（`soft_delete.go`）、租户隔离（`tenant_id.go`）、**树形结构组装**（`tree_path.go`，按 ParentID 构建层级）；`rule/`（system / tenant 策略）与 `interceptor/tenant.go` 提供查询级租户强制——策略层而非字段层。

## 3. MongoDB —— 文档数据库

- **定位**：半结构化数据、灵活 schema、内容管理。
- **文档地图**（[`crud/mongodb/README.md`](../../crud/mongodb/README.md)）：概念对比（关系模型 vs 文档模型的字段映射约定）→ Docker 部署（带密码 / 不带密码 / Compose，含生产注意事项）→ 快速开始（Document 与 DTO 定义、创建 Client、创建 Repository）。

## 4. ClickHouse —— 列式 OLAP

- **定位**：海量日志分析、指标聚合、用户行为分析、实时数仓。
- **文档地图**（[`crud/clickhouse/README.md`](../../crud/clickhouse/README.md)）：Docker 部署（端口说明、Web UI）→ 快速开始（Entity/DTO、创建 Client、创建 Repository、写入数据）。
- **特殊能力**：批量写入（`batch.go`）；Upsert 支持。

## 5. Apache Doris —— 实时分析型数据库

- **定位**：实时 BI 报表、交互式分析。
- **文档地图**（[`crud/doris/README.md`](../../crud/doris/README.md)）：什么是 Doris / 架构概览 / 关键特性 → 快速开始（单条插入、**批量插入（推荐）**、**Stream Load（高性能实时导入）**）。
- **特殊能力**：Upsert 支持。

## 6. Elasticsearch 与 OpenSearch —— 搜索引擎

- **定位**：全文检索、日志分析、聚合（Elasticsearch）；Elasticsearch 开源替代、向量检索、安全分析（OpenSearch）。
- **文档地图**：[`crud/elasticsearch/README.md`](../../crud/elasticsearch/README.md) 与 [`crud/opensearch/README.md`](../../crud/opensearch/README.md) 结构对称——引擎概念与关系库对照、**Mapping 详解**（动态 / 显式 / 严格三大映射类型，常用字段数据类型）、Docker 部署（含 Dashboards 可视面）、快速开始。
- 选这两者前先读 Mapping 章节：字段映射策略决定了 schema 约束与写入自由度的取舍，这是搜索集群运维的第一决策。

## 7. InfluxDB —— 时序数据库

- **定位**：IoT 监控、DevOps 指标、时序数据分析。
- **文档地图**（[`crud/influxdb/README.md`](../../crud/influxdb/README.md)）：Docker 部署（2.x 与 3.x 两种版本线，含 Explorer 管理后台说明）→ 快速开始（DTO 定义、创建 Client、创建 Repository、写入数据）。
- 版本线选择（2.x vs 3.x）在部署章节有专门说明，两代的 API 与存储模型不同，选型后不要混用。

## 8. Cassandra —— 开发中

宽列存储，面向高可用写入与跨数据中心复制。**模块当前处于开发中状态，尚未可用**，文档见 [`crud/cassandra/README.md`](../../crud/cassandra/README.md)。

## 9. 选型决策表

| 需求特征 | 引擎 |
|----------|------|
| 事务性业务数据（订单、账户、配置） | GORM / Ent（按团队对代码生成的接受度二选一） |
| 模型关系复杂、需要 schema 演进治理 | Ent（schema 即代码，编译期校验） |
| 字段结构多变的用户生成内容 | MongoDB |
| 日志/行为流的分析查询、实时数仓 | ClickHouse / Doris（吞吐与生态自行压测取舍） |
| 全文检索、语义/向量检索 | Elasticsearch / OpenSearch |
| 传感器、监控指标等带时间戳的持续写入 | InfluxDB |

跨引擎公共纪律：分析型引擎（ClickHouse / Doris / ES / OS / InfluxDB）**不做事务性主存储**——它们的写入路径与一致性模型为吞吐优化，业务主数据仍落关系库，分析引擎承接投递后的查询面。

## 10. 深入阅读

- 各引擎模块 README（本章各节链接）
- [第 13 章 · 契约层](./13-crud-contracts.md) —— 引擎子包翻译的中立语义来源
- [第 15 章 · 缓存、审计与数据权限](./15-crud-integrations.md) —— 增值层在各引擎上的接入
