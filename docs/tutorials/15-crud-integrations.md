# Go-Wind 插件教程 · 第 15 章：缓存、审计与数据权限集成

> 本篇是 [Go-Wind 插件教程系列](./README.md)的组成部分，覆盖 `crud/cache`、`crud/audit`、`crud/viewer` 三个增值层模块。三者均以 Context 传递为接线方式，与引擎模块解耦（gorm / entgo 的 Repository 提供对 cache 的可选集成，entgo 的 mixin 提供审计字段接入）。

## 1. `crud/cache` —— Cache-Aside 与防击穿

**结构**：`CacheSupport[T]` 是核心封装，组合三个成员——`RedisCacheInterface[T]`（Redis 缓存实现）、`*SingleFlight[T]`（防击穿）、默认 `TTL`。构造形态：

```go
import (
    "github.com/redis/go-redis/v9"
    "github.com/tx7do/go-wind-plugins/crud/cache"
)

redisClient := redis.NewClient(&redis.Options{Addr: "localhost:6379"})

cacheSupport := cache.NewCacheSupport[string](redisClient, 10*time.Minute, nil)
```

**`GetOrLoad` 是唯一取数入口**，实现标准 Cache-Aside 流程：先查缓存，命中即返；未命中则经 singleflight 合并并发回源（同一 key 的并发加载只放行一个请求），加载成功回填缓存后返回，加载失败按配置决定是否缓存空值：

```go
user, err := cacheSupport.GetOrLoad(ctx, "user:123",
    func(ctx context.Context) (*User, error) {
        return db.GetUserByID(ctx, 123)   // 回源加载
    })

// 可选选项（按调用覆盖默认行为）：
v, err := cacheSupport.GetOrLoad(ctx, k, loader, cache.WithTTL(30*time.Minute))
v, err := cacheSupport.GetOrLoad(ctx, k, loader, cache.WithNoCache())        // 强制回源
v, err := cacheSupport.GetOrLoad(ctx, k, loader, cache.WithCacheEmpty(false)) // 不缓存空结果
```

引擎集成侧：gorm / entgo 的 Repository 提供「启用缓存 / 带缓存的查询 / 带缓存的写操作」配置（见各自 README 的缓存功能章节），写操作自动失效对应缓存键。

**文档地图**（[`crud/cache/README.md`](../../crud/cache/README.md)）：核心组件（CacheSupport / RedisCache / SingleFlight / Options 与全部可用选项）→ 高级用法（自定义 Metrics Collector、空值缓存处理、并发场景性能优化）→ 错误处理（`ErrCacheMiss`）→ 最佳实践（TTL 设置、有意义的键设计、写操作后主动失效、命中率监控）→ 完整示例与 API 参考。

## 2. `crud/audit` —— 操作审计

**模型**：`Auditor` 接口 + `Entry` 结构。Entry 记录一次数据操作的完整画像，字段分组为：基础上下文（时间、来源服务、trace 关联）、操作者信息（身份、来源请求）、操作行为（Operation 枚举：创建/读取/更新/删除/…）、数据变更（PreValue / PostValue 前后值）、结果状态（Status 枚举）、扩展字段。

**接线是 Context 驱动**：应用在请求入口把 Auditor 实例放进 context，Repository 层在执行数据操作时取出并记录——业务代码不显式调用审计 API。四个完整示例（GORM Repository 集成审计、中间件自动审计、批量操作审计、应用关闭时刷新缓冲）见模块文档；便捷面包括 `MustFromContext`（取 Auditor，缺失时按策略处理）、`NewNoopAuditor`（显式空实现，用于显式关闭某链路审计）、`SetPreValue` / `SetPostValue`（记录变更前后值）。

**文档地图**（[`crud/audit/README.md`](../../crud/audit/README.md)）：核心概念（Auditor 接口、Entry 字段逐项详解、Operation / Status 枚举）→ 快速开始（四种 Auditor 实现示例：控制台输出 / 文件存储 / 数据库存储 / Elasticsearch）→ Context 传递与便捷方法 → 完整示例 → 最佳实践（异步记录、始终记录错误、记录操作耗时、敏感数据脱敏、PreValue/PostValue 体积控制、TraceID 关联日志）。

引擎侧接入：entgo 的 `mixin/audit.go` 在 schema 层自动填充审计字段。

## 3. `crud/viewer` —— 身份与数据范围

**模型**：`Context` 接口描述「当前是谁」，`DataScope` 描述「当前身份能看到哪些行」。ScopeType 五级：

| ScopeType | 语义 |
|-----------|------|
| `Self` | 仅本人创建的行 |
| `Unit` | 所属组织（部门树）内的行 |
| `User` | 显式指定用户集合的行 |
| `All` | 全量放行 |
| `None` | 禁止访问 |

**接线同样是 Context 驱动**：认证中间件（见[第 10 章](./10-security.md)）解析身份后构造 viewer Context 注入 request context；Repository 侧从 context 提取 Context、执行权限检查（`CanView` / `CanModify` 类判定）、再按数据范围把过滤条件自动并进查询（模块提供「根据数据范围构建 SQL 过滤条件」的示例——行级权限在 WHERE 层生效，业务查询无感知）。`NoopContext` 表示匿名/未授权身份，默认落在最严 scope。

**文档地图**（[`crud/viewer/README.md`](../../crud/viewer/README.md)）：快速开始（创建 / 注入 / 提取 Context、权限检查、数据范围控制、租户隔离、审计联动、NoopContext）→ API 参考（Context 接口、DataScope、ScopeType 枚举、Context 管理函数）→ 五种 ScopeType 的逐一详解与语义边界 → 最佳实践（中间件注入模式、Repository 自动应用、权限检查装饰器、多数据范围组合、平台管理员旁路、系统任务跳过检查、审计联动、组织树缓存）→ 与 audit / gorm / entgo 的集成章节。

## 4. 三者的协作形态

典型组装：认证中间件解析身份 → 构造 viewer Context 与 Auditor 注入 context → 业务 handler 调 Repository → Repository 侧：viewer 的数据范围变成查询过滤（WHERE 子句收敛可见行），audit 的 Entry 记录本次操作，cache 的 GetOrLoad 承接读路径。三层都作用于同一请求 context，互不显式调用——这也是它们能对所有引擎统一生效的原因。

三个提醒：数据范围过滤是**行级**防线，字段级裁剪靠 FieldMask（[第 13 章](./13-crud-contracts.md)）；审计记录进生产存储前按最佳实践做脱敏与体积控制；缓存键必须包含租户/归属维度，否则跨租户串数据。

## 5. 深入阅读

- [`crud/cache/README.md`](../../crud/cache/README.md) · [`crud/audit/README.md`](../../crud/audit/README.md) · [`crud/viewer/README.md`](../../crud/viewer/README.md)
- [第 12 章 · CRUD 总览](./12-crud-overview.md) —— 家族分层与依赖结构
- [第 10 章 · 安全](./10-security.md) —— 身份来源（认证中间件）与授权的边界
