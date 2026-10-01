# Go-Wind 插件教程 · 第 16 章：工作流与分布式事务

> 本篇是 [Go-Wind 插件系列教程](./README.md)的组成部分。工作流引擎的接入形态与第 7、8 章的消息代理、传输层类似：四个引擎模块对接各家工作流平台的客户端 SDK。

## 1. 接口与矩阵

四个引擎的工作流操作参数与返回值类型互不兼容，因此本层**只提取最小公共接口** `workflow.Client`（仅 `Close() error`）——即各模块构造出的客户端都支持关闭，其余操作直接使用各引擎的原生类型，不做二次抽象：

| 插件 | 模块路径 | 框架 |
|------|---------|------|
| Argo Workflows | `github.com/tx7do/go-wind-plugins/workflow/argo` | Argo Workflows REST API |
| Conductor | `github.com/tx7do/go-wind-plugins/workflow/conductor` | conductor-sdk/conductor-go |
| GoWorkflows | `github.com/tx7do/go-wind-plugins/workflow/goworkflows` | cschleiden/go-workflows |
| Temporal | `github.com/tx7do/go-wind-plugins/workflow/temporal` | temporal.io/sdk |

## 2. 引擎速览

- **[Argo Workflows](../../workflow/argo/README.md)**：Kubernetes 原生的批处理工作流引擎。模块文档覆盖：创建客户端、提交 Workflow、查询执行状态、管理操作。适合 K8s 环境下的容器化批处理 DAG。
- **[Conductor](../../workflow/conductor/README.md)**：Netflix 出品的编排引擎，用 JSON DSL 定义流程。文档覆盖：创建客户端、启动工作流、定义任务 Worker（人类任务/自动化任务）、管理操作。适合含人工环节的长流程编排。
- **[Temporal](../../workflow/temporal/README.md)**：持久化执行（durable execution）的代表性引擎，代码即工作流。文档覆盖：核心概念、Docker 部署（本地开发环境）、创建客户端、异步/同步执行工作流、启动 Worker。适合对状态可靠性与重试语义要求最高的核心业务流程。
- **[GoWorkflows](../../workflow/goworkflows/README.md)**：嵌入式的工作流引擎实验项目。文档见模块 README。

选型的首要判据是**部署形态**：Argo 绑定 K8s；Temporal / Conductor 是独立服务集群；GoWorkflows 嵌入进程。其次是执行语义（持久化执行 vs 任务编排）与团队对 DSL / 代码式定义的接受度。

## 3. 分布式事务家族（transaction/*）

与工作流层同构：根模块 `github.com/tx7do/go-wind-plugins/transaction` 只定义最小公共接口 `Client`（仅 `Close() error`）——各事务模式的操作签名互不兼容（Saga 步骤、TCC 分支、XA 资源均无法统一抽象），全部留在具体类型上。五个子模块覆盖从事件最终一致到数据库两阶段提交的谱系：

| 模块 | 路径 | 一致性模型 | 外部组件 |
|------|------|------------|----------|
| DTM | `github.com/tx7do/go-wind-plugins/transaction/dtm` | Saga / TCC / XA / 二阶段消息（引擎侧全量） | DTM Server |
| Outbox | `github.com/tx7do/go-wind-plugins/transaction/outbox` | 二阶段消息（事务性发件箱） | 无（业务库 + broker） |
| Saga | `github.com/tx7do/go-wind-plugins/transaction/saga` | 进程内补偿 | 无 |
| TCC | `github.com/tx7do/go-wind-plugins/transaction/tcc` | 业务层预留 | 无 |
| XA | `github.com/tx7do/go-wind-plugins/transaction/xa` | 数据库两阶段提交 | 无 |

表中「一致性模型」「外部组件」两列取自 outbox 模块 README 的「与其他事务模式的关系」对照表。各模块速览：

- **[DTM](../../transaction/dtm/)**：对接 [DTM](https://dtm.pub) 框架的引擎侧全量封装（client / msg / saga / tcc / xa 各源文件对应各模式）。该模块无独立 README，接入细节见模块源码与 DTM 官方文档。
- **[Outbox](../../transaction/outbox/README.md)**：事务性发件箱——`Enqueue` 在业务本地事务内写入发件箱表（事件与业务数据原子提交），`Relay` 后台轮询把已提交事件发布到任意 [`broker.Broker`](./07-broker.md) 实现；消费端 `Inbox` 按 `(handler, eventID)` 去重，`Barrier` 把业务写库与去重记录放进同一事务以达成恰好一次。投递语义是 at-least-once（Relay 崩溃后由租约回收重投，靠消费端去重吸收重复）；表结构、多实例并发（`SELECT ... FOR UPDATE SKIP LOCKED`）、重试与死信、清理选项等运维细节见模块 README。
- **[Saga](../../transaction/saga/README.md)**：进程内泛型补偿编排——`saga.Run(ctx, state, steps...)` 按序执行各步骤的 `Do`，任一失败时对已完成步骤逆序执行 `Undo`，补偿错误与原始错误经 `errors.Join` 一并返回；`Undo` 必须幂等（崩溃恢复或重试可能重复调用）。
- **[TCC](../../transaction/tcc/README.md)**：进程内泛型预留-确认-释放（`Try` / `Confirm` / `Cancel`）。两个不变量：**空补偿**（任一 Try 失败时 Cancel 会作用于所有已尝试的参与方，因此 Cancel 必须容忍「Try 从未成功」的空状态）；**Confirm 不回头**（进入 Confirm 阶段后绝不再 Cancel，Confirm 错误只经 `errors.Join` 汇总返回，交由重试与告警兜底）。
- **[XA](../../transaction/xa/README.md)**：MySQL / MariaDB 的多库两阶段提交（`xa.Run`，每资源一条专用连接执行 `XA START/END/PREPARE/COMMIT`）。孤儿事务恢复：`xa.Pending` 以 `windxa-` 前缀过滤残留 XID，`CommitPending` / `RollbackPending` 拒绝前缀与字符集不符的 XID（防注入）；单元测试基于 go-sqlmock 断言语句序列，接入生产前需以真实双库环境验证。

选型顺序（outbox README 的原文立场）：**能用本地事务就不用分布式**；最终一致够用选 outbox；进程内回滚选 saga、预留-确认选 tcc；多库强一致短事务选 xa；跨进程持久化编排选 workflow/temporal（本章第 1、2 节）；以上都不满足、且接受部署外部事务服务器时，再回到 dtm。与工作流层的边界：**工作流管业务流程的编排，事务框架管原子性**——一次长流程里的某个跨服务写入步骤需要事务保证时，二者组合使用。

## 4. 深入阅读

- [`workflow/argo/README.md`](../../workflow/argo/README.md) · [`workflow/conductor/README.md](../../workflow/conductor/README.md) · [`workflow/temporal/README.md`](../../workflow/temporal/README.md) · [`workflow/goworkflows/README.md](../../workflow/goworkflows/README.md)
- [`transaction/dtm/`](../../transaction/dtm/) · [`transaction/outbox/README.md`](../../transaction/outbox/README.md) · [`transaction/saga/README.md`](../../transaction/saga/README.md) · [`transaction/tcc/README.md`](../../transaction/tcc/README.md) · [`transaction/xa/README.md`](../../transaction/xa/README.md) · [DTM 官方文档](https://dtm.pub)
- [根 README · 工作流引擎矩阵](../../README.md#工作流引擎)
