# 上游同步协议（go-wind-plugins · 双上游轴）

本仓有两条独立同步轴，方法论相同（归一化 sed + 逐文件 diff），规则集不同：

- 第一同步轴：kratos-transport（框架传输层与中间件类模块，含大量结构性丢弃/保留规则）
- 第二同步轴：go-crud（数据访问层，crud/ 忠实镜像，固定差异仅三条）

---

# 第一同步轴：kratos-transport → go-wind-plugins

上游仓 `/d/GoProject/kratos-transport`（git，HEAD 为最新修复后状态）与分叉仓
`/d/GoProject/go-wind-plugins` 同源。你的任务：把上游的全仓审计修复（并发/生命周期/
消息可靠性/配置接线等）移植到分叉的对应文件。

## 归一化 diff 方法

对每个目标文件（相对路径两仓相同）：

```bash
cd /d/GoProject/kratos-transport
git show HEAD:<path> | sed -f /tmp/rewrite.sed | diff - /d/GoProject/go-wind-plugins/<path>
```

`/tmp/rewrite.sed` 内容（已存在，若丢失按此重建）：
```
s|github.com/tx7do/kratos-transport/tracing|github.com/tx7do/go-wind-plugins/tracer/otlp|g
s|github.com/tx7do/kratos-transport|github.com/tx7do/go-wind-plugins|g
s|github.com/go-kratos/kratos/v2/encoding|github.com/tx7do/go-wind-plugins/encoding|g
s|github.com/go-kratos/kratos/v2/transport|github.com/tx7do/go-wind/transport|g
s|github.com/go-kratos/kratos/v2/errors|github.com/tx7do/go-wind/errors|g
s|github.com/go-kratos/kratos/v2/log|github.com/tx7do/go-wind/log|g
```

无差异 → 该文件完成。

## 差异 hunks 的分类与处置

diff 的 `<` 行为上游独有、`>` 行为分叉独有。逐块判断：

**分叉独有、必须保留的（">" 行）：**
- metrics 埋点：`"github.com/tx7do/go-wind-plugins/metrics"` import、
  `m metrics.Metrics` 字段、`WithMetrics` option、`wrapHandler` 整个函数、
  `handler = s.wrapHandler(topic, handler)` 调用行
- `func (s *Server) Endpoint() string { return "" }`（分叉框架的接口形态）
- 分叉的 `kind.go`（Kind 常量所在，若分叉把常量放在 server.go 顶部，保持其位置）
- 分叉的 `subscribe_option.go`（等价上游 transport/options.go，不动）
- 中文品牌字符串（如 wind-group/wind-consumer 替代 kratos-group 等）
- 其它与上游修复不冲突的分叉自有逻辑（逐行判断，宁可保留并报告，不可静默丢弃）

**上游独有、必须丢弃的（"<" 行，即不移植）：**
- `keepalive` 子系统的一切：`transport/keepalive` import、`keepaliveServer` 字段、
  `keepalive.NewServer(...)`、`ka.Start/Stop` 启停块、`WithServiceKind`
- `kratosTransport "github.com/txdo/go-wind/transport"` import 与
  `var ( _ kratosTransport.Server = ...; _ kratosTransport.Endpointer = ... )` 断言块
- `func (s *Server) Endpoint() (*url.URL, error)`（keepalive 委托版 Endpoint）及
  仅为它服务的 `net/url` import
- `github.com/go-kratos/kratos/v2/selector` 相关（上游 Transporter 桩残留）

**上游独有、必须移植的（"<" 行，即上游修复本体）：**
以上之外的一切。已知修复模式（上游 2026-09-20 审计批次）：
- Start：`s.err` 粘滞错误处理；Init/Connect 失败赋 `s.err`；Connect 成功后才
  `started.Store(true)`（先置位再注册订阅）；Start 失败后允许重试（Stop 的
  `!started` 分支清 `s.err`）
- Stop：`started.Store(false)` 置先；持锁快照订阅表再遍历退订；
  保留 `subscriberOpts`（Start→Stop→Start 重新注册）；Disconnect 后清 `s.err`
- RegisterSubscriber（公有）：锁内检查 started；未启动 → 记录到
  `subscriberOpts` 延迟到下次 Start；已启动 → 锁外调用 doRegisterSubscriber
- 泛型 RegisterSubscriber[T]：nil 检查（event/message/body）、`case T` 值类型分支、
  expectedType 错误消息
- doRegisterSubscriber：锁内 started 复查、旧订阅退订（exists 分支）、
  stopped 竞态退订、订阅参数记录
- doRegisterSubscriberMap：持锁取出并清空缓存表
- broker 驱动侧：NewBroker 的 `if l := broker.LoggerFromOptions(&options); l != nil { SetLogger(l) }`
  接线（分叉的 logger.go 已有 SetLogger，直接可用）；
  毒消息 ErrorHandler 通知；Publish 用调用方 ctx；退订幂等；nil pool 守卫；
  断线重连后地址保持；PING 独立连接；PONG 静默；PEL 认领（XAUTOCLAIM）；
  事件转换补 Key/ID；Request 委托 `broker.GenericRequest`；等等
- 上游配置项注释新增的"注意：当前驱动不消费 X"说明行 → 移植

## 产出方式

- 若除"必须保留"与"必须丢弃"外无其它差异 → 产出 = 上游归一化内容，去掉丢弃项，
  加回保留项（参照分叉现行文件的对应块）
- 若分叉文件结构大幅重写（如 sse）→ 读上游修复 diff，把修复语义手工移植进分叉结构
- 新增文件（上游有、分叉无，非测试基建）→ 归一化后整体落盘（丢弃项同样去掉）

## 禁改清单

example_test.go、it_guard_test.go、it_helper_test.go、transport.go、transport_test.go、
kind.go、subscribe_option.go、logger.go（已注入化完成，勿动）、README、go.mod、go.sum。
offline 单测（不依赖真实 broker 的 *_test.go 新增）可一并移植；纯集成测试文件跳过。
分叉自有 example_test.go（2026-10-01 晚批建立，全仓 202 模块）为分叉保留项：上游
example_test.go 不移植，分叉同名文件不得被覆盖或删除。

## 验证

每完成一个模块：
```bash
cd /d/GoProject/go-wind-plugins/<module> && go build ./... && go vet ./...
```
必须通过。失败时修复（在上述规则范围内）；超出范围的问题记录到报告。

## 报告格式（最终输出）

逐文件一行：`<path>: PORTED(wholesale|merged|semantic) | SKIPPED(<reason>) | UNCHANGED`
加一段编译状态与遇到的异常。

---

# 第二同步轴：go-crud → go-wind-plugins/crud

上游仓 `/d/GoProject/go-crud`（git，HEAD 为最新状态）是数据访问层工具库，与镜像
`/d/GoProject/go-wind-plugins/crud/*` **模块一一对应**：api / audit / cache / cassandra /
clickhouse / doris / elasticsearch / entgo / gorm / influxdb / mongodb / opensearch /
pagination / viewer，共 14 个。上游根目录的 interface.go（空壳）不镜像。上游各模块
自带类内 sibling replace（`=> ../api` 等），相对路径在本仓布局下语义不变，归一化后
直接成立，无需增删。

本轴为**忠实镜像**：无第一轴的"必须丢弃/禁改"清单，上游对模块源码的一切改动均移植。

## 归一化 diff 方法

对每个目标文件（相对路径 = `crud/<module>/<path>` 对上游 `<module>/<path>`）：

```bash
cd /d/GoProject/go-crud
git show HEAD:<module>/<path> | sed -f /tmp/rewrite_crud.sed | diff - /d/GoProject/go-wind-plugins/crud/<module>/<path>
```

`/tmp/rewrite_crud.sed` 内容（若丢失按此重建；**两条规则顺序不可颠倒**，URL 规则
必须先行，否则 `tree/main/<路径>` 形态会被通用规则改写为死链）：
```
s|github.com/tx7do/go-crud/tree/main/|github.com/tx7do/go-wind-plugins/tree/main/crud/|g
s|github\.com/tx7do/go-crud|github.com/tx7do/go-wind-plugins/crud|g
```

第一条只处理指向上游仓库内容的 URL（`…go-crud/tree/main/<module>/…` →
`…go-wind-plugins/tree/main/crud/<module>/…`）；第二条处理模块路径（module 行、
import、require、replace）。上游对 `github.com/tx7do/go-wind-plugins/encoding*` 与
`github.com/tx7do/go-wind` 的引用是既有的发布版依赖，**不在重写范围**，原样保留。

## 镜像侧固定差异（同步时保持，不算待移植差异）

归一化后与上游仍会存在的 diff，全部由以下五条固定规则解释（2026-10-02 增补第四、五条）：

1. **类内互引 require 版本固定 `v0.0.1`**——上游用其自身发布序列的版本号
   （v0.0.7 / v0.0.16 等），本仓所有模块统一打 `v0.0.1` 标签，互引只有对齐到
   v0.0.1 才可解析。同步上游 go.mod 时，类内 require 行保持镜像侧的 v0.0.1，
   不带回上游版本号。
2. **`replace github.com/tx7do/go-crud => ../` 一律删除**（归一化后形如
   `replace github.com/tx7do/go-wind-plugins/crud => ../`）——它指向上游根模块，
   镜像没有 crud 根模块，留下即悬空。上游有 11 个模块带此行。
3. **go.work（本仓根文件，非上游文件）**：`use` 含 14 个 `./crud/<module>` 路径；
   `go` 行须 ≥ 各成员 go 指令最大值（当前 1.26.4，由 entgo 的 go 指令决定）。
   上游新增/移除模块或调整 go 指令时，同步维护本仓 go.work。
4. **镜像测试文件保留分叉侧 KRATOS_IT 门控 hunk**（`skipWithoutIntegration`/
   `os.Getenv("KRATOS_IT")` 跳过块及配套注释与 import，来自 2026-10-01 的全仓门控
   提交）。上游无门控版本的测试文件不移植覆盖；含该类 hunk 的 diff 按固定差异跳过。
5. **镜像侧对 sed 路径改写导致 import 块字母序翻转的文件保持 gofmt 排序**。
   `go-crud` 改写为 `go-wind-plugins/crud` 后，与 `go-utils`/`go-wind` 等路径的相对
   字母序翻转，gofmt 要求该 import 行换位；上游 blob 按其自身路径排序是干净的，镜像
   因此与上游 blob 存在**单行 import 位置差**——属预期固定差异，同步时跳过该位置差
   （不回写 blob 顺序），无需上游修复。同步流程要求：**移植任何 .go 文件后必跑
   `gofmt -l`**；若脏且差异仅为 import 位置（本类翻转），`gofmt -w` 归一化并把该
   文件记入本条清单。当前实例（2026-10-02，13 个）：gorm/repository_cache_test.go；
   influxdb/{field/field_selector.go,filter/filter_processor.go,sorting/structured_sorting.go}；
   mongodb/{field/field_selector.go,filter/filter_processor.go,repository_test.go,sorting/structured_sorting.go}；
   opensearch/{field/field_selector.go,opensearch_client_test.go,sorting/structured_sorting.go}；
   pagination/{filter/operator_converter_test.go,sorting/order_by_string_converter.go}。

另：镜像侧存在上游没有的额外文件（分叉自建单测、example_test.go、带门控的测试变体）。
协议的 diff 方向（上游→镜像）天然不涉及它们，同步不得删除或上报为差异。

## crud/api 生成代码（gen/）的特殊流程

`api/gen/go/**/*.pb.go` 的 raw descriptor 内嵌 go_package 与文档 URL 字符串，
**带 protobuf 长度前缀，绝不可直接 sed**（字符串变长即损坏 descriptor，包 init 即
panic）。同步流程：

1. 先按上述 sed 归一化 `api/protos/**` 与 `api/buf.gen.yaml`（proto 的
   go_package 是明文 option，buf 的 `go_package_prefix.default` 是明文配置，均可
   安全 sed）；
2. 在 `crud/api` 下执行 `buf generate`（本机 buf 与 protoc-gen-go 在位；依赖
   buf.build/gnostic/gnostic，需网络）整体重新生成 `gen/`；
3. `gen/` 输出**不与上游 pb.go 做逐行 diff**，一律以再生成结果为准。

## 验证

每完成一个模块：
```bash
cd /d/GoProject/go-wind-plugins/crud/<module> && go build ./... && go vet ./...
```
必须通过。失败时区分：镜像侧问题 → 在上述规则范围内修复；上游固有问题 → 记入
报告，宜回上游仓修复后重新同步。

