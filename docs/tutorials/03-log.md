# Go-Wind 插件教程 · 第 3 章：日志（Log）

> 本篇是 [Go-Wind 插件系列教程](./README.md)的组成部分。日志插件实现 [`github.com/tx7do/go-wind-plugins/log`](../../log/) 根模块定义的 `Logger` 接口，经 go-wind 框架应用选项 `WithLogger` 注入。

## 1. 这一层解决什么问题

服务需要打日志，而「日志最终去哪」是部署决定的：本地终端、文件、集中式日志平台。日志层把输出动作抽象成统一接口，引擎插件对接各后端；业务代码只调用 `Info` / `Error` 等方法，换后端零改动。

## 2. 接口架构

| 接口 | 方法 | 说明 |
|------|------|------|
| `Logger` | `Debug / Info / Warn / Error(ctx, msg, keyvals...)` | 四级日志输出，`keyvals` 为成对的结构化字段 |
| `Logger` | `With(keyvals...) Logger` | 返回携带固定上下文字段的子 Logger |
| `Logger` | `Enabled(Level) bool` | 级别开关判断，用于跳过昂贵的日志构造 |

注意两点：其一，接口是**结构化日志**形态——字段以键值对传入，而不是让调用方拼字符串，这使得同一份日志既能落到人类可读的控制台，也能无损进入 ES/Loki 这类检索平台；其二，`With` 的典型用法是在请求入口把 trace-id、请求-id 绑进子 Logger，随后整条链路的日志自动携带（与[第 6 章追踪](./06-tracer.md)的上下文传播配合）。

## 3. 插件矩阵

| 插件 | 模块路径 | 引擎 |
|------|---------|------|
| Aliyun SLS | `github.com/tx7do/go-wind-plugins/log/aliyun` | 阿里云日志服务 SLS |
| Charm | `github.com/tx7do/go-wind-plugins/log/charm` | charmbracelet/log |
| CloudWatch | `github.com/tx7do/go-wind-plugins/log/cloudwatch` | AWS CloudWatch Logs |
| Fluent | `github.com/tx7do/go-wind-plugins/log/fluent` | Fluentd |
| Glog | `github.com/tx7do/go-wind-plugins/log/glog` | golang/glog |
| Hclog | `github.com/tx7do/go-wind-plugins/log/hclog` | hashicorp/go-hclog |
| Logrus | `github.com/tx7do/go-wind-plugins/log/logrus` | sirupsen/logrus |
| Loki | `github.com/tx7do/go-wind-plugins/log/loki` | Grafana Loki |
| Phuslu | `github.com/tx7do/go-wind-plugins/log/phuslu` | phuslu/log |
| Sentry | `github.com/tx7do/go-wind-plugins/log/sentry` | getsentry/sentry-go |
| Tencent CLS | `github.com/tx7do/go-wind-plugins/log/tencent` | 腾讯云日志服务 CLS |
| Zap | `github.com/tx7do/go-wind-plugins/log/zap` | uber-go/zap |
| Zerolog | `github.com/tx7do/go-wind-plugins/log/zerolog` | rs/zerolog |

## 4. 接入示例：zap

zap 引擎的构造函数直接接收一个原生 `*zap.Logger`，由你先组装好 zap 的编码器与输出目标：

```go
import (
    "go.uber.org/zap"
    zaplog "github.com/tx7do/go-wind-plugins/log/zap"
    wind "github.com/tx7do/go-wind"
)

zlog, _ := zap.NewProduction()          // 或按需自定义 zap.Config
logger := zaplog.NewZapLogger(zlog)     // 包装为统一 Logger

wind.New(wind.WithLogger(logger))       // 注入框架，业务代码经框架 Logger 打日志
```

其余引擎的构造形态一致：传入引擎原生客户端或配置，返回统一 `Logger`。框架内的传输层访问日志中间件（`transport/http/middleware/logging` 等）也消费同一个注入的 Logger，请求日志与业务日志因此汇入同一后端、同一格式。

## 5. 选型建议与已知问题

| 场景 | 建议 |
|------|------|
| 高吞吐生产服务 | `zap` / `zerolog`（零分配设计） |
| 本地开发终端 | `charm`（样式化输出）/ `phuslu`（轻量） |
| 集中式日志平台 | `loki` / `fluent` / `cloudwatch` / `tencent` 按平台选 |
| 错误聚合与告警 | `sentry`（Error 级别事件上报） |
| 存量代码库绑定旧日志库 | `glog` / `hclog` / `logrus` 兼容层 |

**已知问题**：`log/aliyun` 当前因阿里云 SDK 与本仓工具链不兼容而构建失败（2026-09-30 基线确认），选用前先确认构建环境。其余 12 个引擎均随构建验证通过。

## 6. 深入阅读

- [`log/zap`](../../log/zap/) —— zap 引擎源码
- [根 README · 日志矩阵](../../README.md#日志log)
- [第 6 章 · 分布式追踪](./06-tracer.md) —— trace-id 与日志字段的联动
