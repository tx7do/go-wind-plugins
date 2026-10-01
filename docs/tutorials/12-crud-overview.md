# Go-Wind 插件教程 · 第 12 章：CRUD 数据访问层总览

> 本篇是 [Go-Wind 插件系列教程](./README.md)的组成部分，覆盖 `crud/` 插件分类。该家族是上游 [go-crud](https://github.com/tx7do/go-crud) 的忠实镜像（仓库同步协议第二轴），上游为 canonical 开发地。

## 1. 这一层解决什么问题

业务代码里最大的样板来源是数据访问：八种存储引擎，各自一套连接管理、查询构造、分页翻译、结果映射。CRUD 家族用**一套泛型 `Repository` / `Client` 抽象**盖住它们——业务代码面向接口与 DTO 编程，引擎模块负责把统一语义翻译成各家的查询语言。配套的共享设施（分页、缓存、审计、数据权限）以同一套契约横切所有引擎。

## 2. 家族模块图

**引擎模块**（各实现统一 Repository 语义，模块 README 均为千行级完整文档）：

| 模块 | 定位 |
|------|------|
| [`crud/gorm`](../../crud/gorm/README.md) | GORM：关系型 ORM，支持 MySQL / PostgreSQL / SQLite / SQL Server 等 |
| [`crud/entgo`](../../crud/entgo/README.md) | Ent：代码生成型关系型 ORM（Facebook 开源），编译期类型安全 |
| [`crud/mongodb`](../../crud/mongodb/README.md) | MongoDB 文档数据库 |
| [`crud/clickhouse`](../../crud/clickhouse/README.md) | ClickHouse 列式 OLAP |
| [`crud/doris`](../../crud/doris/README.md) | Apache Doris 分析型数据库（MySQL 协议，含 Stream Load） |
| [`crud/elasticsearch`](../../crud/elasticsearch/README.md) | Elasticsearch 全文检索与聚合 |
| [`crud/opensearch`](../../crud/opensearch/README.md) | OpenSearch（Elasticsearch 开源分支） |
| [`crud/influxdb`](../../crud/influxdb/README.md) | InfluxDB 时序数据库 |
| `crud/cassandra` | Cassandra 宽列存储——**开发中，尚未可用** |

**共享设施模块**（被引擎模块依赖）：

| 模块 | 角色 |
|------|------|
| [`crud/api`](../../crud/api/README.md) | Protobuf 契约定义与 buf 生成代码（分页请求、过滤表达式、排序、字段掩码） |
| [`crud/pagination`](../../crud/pagination/README.md) | 分页器、过滤器与排序格式转换的引擎无关实现（[第 13 章](./13-crud-contracts.md)） |
| [`crud/cache`](../../crud/cache/README.md) | Cache-Aside 缓存与 SingleFlight 防击穿封装（[第 15 章](./15-crud-integrations.md)） |
| [`crud/audit`](../../crud/audit/README.md) | 审计日志接口、Context 注入与变更记录（[第 15 章](./15-crud-integrations.md)） |
| [`crud/viewer`](../../crud/viewer/README.md) | 身份上下文与五级数据范围（行级数据权限）（[第 15 章](./15-crud-integrations.md)） |

## 3. 引擎能力对照

来自上游 README 的引擎定位表：

| 引擎 | 类型 | 适用场景 |
|------|------|----------|
| GORM | 关系型 ORM | MySQL、PostgreSQL、SQLite、SQL Server 等主流关系库的通用业务存储 |
| Ent | 关系型 ORM（代码生成） | 同上，换取编译期类型安全与 schema 演进管理 |
| MongoDB | 文档数据库 | 半结构化数据、灵活 schema、内容管理 |
| ClickHouse | 列式 OLAP | 海量日志分析、指标聚合、用户行为分析、实时数仓 |
| Apache Doris | 列式 OLAP | 实时 BI 报表、交互式分析（Stream Load 高速写入） |
| Elasticsearch | 搜索引擎 | 全文检索、日志分析、高亮、聚合分析 |
| OpenSearch | 搜索引擎 | Elasticsearch 开源替代、向量检索、安全分析 |
| InfluxDB | 时序数据库 | IoT 监控、DevOps 指标、时序数据分析 |
| Cassandra | 宽列数据库 | 高可用写入、跨数据中心复制（开发中） |

跨引擎的横切能力（以各引擎模块文档为准）：**Upsert**（INSERT ON CONFLICT）在 GORM / ClickHouse / Doris 原生支持；**树形查询**（按 ParentID 组装层级）为 Ent 模块内置；**读写分离、自动迁移、连接池配置**见 GORM 模块文档。

## 4. 家族内部依赖结构

从模块依赖（类内互引）可见分层——引擎模块永远依赖契约层，可选依赖增值层：

```
crud/api（契约） ──┐
crud/pagination ──┼──> 各引擎模块
crud/viewer ──────┘        │
                           ├──(可选) crud/cache    [gorm, entgo]
                           └──(可选) crud/audit    [entgo]
```

多租户隔离（tenant_id mixin / 租户强制过滤）作为引擎侧 mixin 提供，存在于 gorm / entgo / mongodb / clickhouse / doris 的模块内（各引擎 `mixin/` 与 `tenant_enforce.go`）——租户字段的注入与过滤在 Repository 层自动完成，业务无感知。

## 5. DTO 与 Entity 的分离

引擎模块强制两条数据形态：**Entity** 是与存储 schema 一一映射的结构体（表结构 / 文档结构 / mapping），**DTO** 是 Protobuf 生成的传输结构（即 [第 5 章](./05-encoding.md) proto 编解码所作用的类型）。两者之间的双向映射由 `go-utils/mapper` 完成，Repository 的泛型参数同时绑定两者——`Repository[TEntity, TDTO]` 形态让「存的是什么」与「传出的是什么」在类型上就分开，字段裁剪、字段掩码的执行点也因此明确。

## 6. 为什么这个家族叫 `crud/`

命名空间隔离是有意为之：本仓根下已有 `cache/`（通用缓存插件，[第 11 章](./11-cache.md)），而本家族的 `crud/cache` 是数据访问层的 Cache-Aside 封装——同名不同义。`crud/` 前缀让家族整体与上游 go-crud 保持 1:1 路径映射（同步协议的归一化规则因此只有一条 sed），同时避免与既有分类撞名。

## 7. 阅读顺序建议

1. 本章建立全貌；
2. [第 13 章](./13-crud-contracts.md)：契约层——无论用哪个引擎，分页/过滤/排序的语义都在这里定义；
3. [第 14 章](./14-crud-engines.md)：按目标引擎进入对应的引擎模块 README（每个都是千行级完整文档，含 Docker 部署）；
4. [第 15 章](./15-crud-integrations.md)：缓存、审计、数据权限三个增值层的接入。

## 8. 深入阅读

全部家族模块 README：[`api`](../../crud/api/README.md) · [`pagination`](../../crud/pagination/README.md) · [`pagination/filter`](../../crud/pagination/filter/README.md) · [`cache`](../../crud/cache/README.md) · [`audit`](../../crud/audit/README.md) · [`viewer`](../../crud/viewer/README.md) · [`gorm`](../../crud/gorm/README.md) · [`entgo`](../../crud/entgo/README.md) · [`mongodb`](../../crud/mongodb/README.md) · [`clickhouse`](../../crud/clickhouse/README.md) · [`doris`](../../crud/doris/README.md) · [`elasticsearch`](../../crud/elasticsearch/README.md) · [`opensearch`](../../crud/opensearch/README.md) · [`influxdb`](../../crud/influxdb/README.md)

另见 [根 README · 数据访问层矩阵](../../README.md#数据访问层crud)。
