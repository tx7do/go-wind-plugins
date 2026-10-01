# Go-Wind 插件教程 · 第 13 章：分页、过滤与排序契约

> 本篇是 [Go-Wind 插件系列教程](./README.md)的组成部分，覆盖 `crud/api`（契约定义）与 `crud/pagination`（引擎无关转换）两个模块。这两个模块是所有 CRUD 引擎共享的查询语义层。

## 1. 三层结构

查询语义的流转分三层，每层职责单一：

```
传输层 DTO            转换层（crud/pagination）              引擎层
PagingRequest   ──>   Paginator + FilterConverter      ──>   各引擎方言的
FilterExpr            + OrderByStringConverter               LIMIT/OFFSET、WHERE、ORDER BY
Sorting               （产出引擎无关的中间结构）              （gorm/pagination、clickhouse/pagination/...）
FieldMask
```

- **`crud/api`**：以 Protobuf 定义 `PagingRequest` / `PaginationRequest` / `PaginationResponse`、`FilterExpr`、`Sorting`、`FieldMask` 消息，经 buf 生成 Go 代码（`crud/api/gen/go/pagination/v1`）。它是 API 层的数据结构，不含任何引擎语义。
- **`crud/pagination`**：把上述 DTO 翻译成引擎无关的中间结构。转换器分两个语系：`QueryStringConverter`（JSON 字符串语法）与 `FilterStringConverter`（Google AIP-143 语法）；排序对应 `OrderByStringConverter`。
- **引擎层**：各引擎模块内的 `pagination/` 子包（如 `crud/gorm/pagination/offset_paginator.go`）把中间结构落成方言。引入新引擎时只需实现这一层。

## 2. 契约在 API 中的使用

业务 proto 里导入契约并声明分页字段：

```protobuf
syntax = "proto3";
package myservice;
import "pagination/v1/pagination.proto";

message ListUsersRequest {
  pagination.PaginationRequest pagination = 1;
}
message ListUsersResponse {
  pagination.PaginationResponse response = 1;
}
```

服务端代码构造查询参数（`PagingRequest` 同时携带分页、排序与字段掩码）：

```go
import (
    paginationV1 "github.com/tx7do/go-wind-plugins/crud/api/gen/go/pagination/v1"
    "google.golang.org/protobuf/types/known/fieldmaskpb"
)

req := &paginationV1.PagingRequest{
    Page:     &page,           // uint32 页码
    PageSize: &pageSize,       // uint32 页大小
}
req.Sorting = []*paginationV1.Sorting{
    {Field: "created_at", Direction: paginationV1.Sorting_DESC},
}
req.FieldMask = &fieldmaskpb.FieldMask{
    Paths: []string{"id", "name", "email"},   // 只取需要的字段
}
```

## 3. 分页的四种模式

契约层定义四种分页形态，语义与取舍（完整对比表见 [`crud/pagination/README.md`](../../crud/pagination/README.md) 的「分页方式对比」）：

| 模式 | 机制 | 适用 |
|------|------|------|
| **Page-Based** | 页码 × 页大小换算 offset | 传统 Web 表格，数据集有限 |
| **Offset-Based** | 直接给 offset/limit | 与 page 模式同源，供已有 offset 语义的调用方 |
| **Token-Based（游标）** | 不透明续读令牌，结果集按稳定排序增量续读 | 无限滚动、大数据集、实时流——**推荐**，无深分页代价 |
| **No Paging** | 全量返回 | 配置、字典类小数据集 |

引擎无关的分页器实现：`PagePaginator` / `OffsetPaginator` / `TokenPaginator`（均在 `crud/pagination/paginator/`，每个引擎模块内的 `pagination/` 子包对其做方言适配）。**排序对 Token 模式是硬要求**——游标的正确性依赖结果集全序，契约层也始终建议显式指定排序。

## 4. 过滤表达式

`FilterExpr` 支持结构化的多层 AND/OR 嵌套与日期时间部件提取（DatePart），操作符覆盖比较、范围、字符串匹配、NULL 检查等类别。两种输入语法：

- **JSON 语法**：`{"field1":"val1", "field2___icontains":"val2"}`——字段名 + `___` + 操作符后缀，天然适配前端表单序列化；
- **Google AIP 语法**：`name = "alice" AND create_time > "2024-01-01"`——AIP-143 定义的类 SQL 谓词。

两种语法经各自的转换器（`QueryStringConverter` / `FilterStringConverter`）落到同一棵中间表达式树，也可以直接从 `PagingRequest` 自动转换。操作符的完整清单（查找类型规范、日期时间提取类）在 [`crud/pagination/filter/README.md`](../../crud/pagination/filter/README.md)——该文档同时给出五类嵌套组合（纯 AND / 纯 OR / AND 嵌 OR / OR 嵌 AND / 多层）的解析示例。

**注入防护是这一层的核心承诺**：转换产物是参数化的查询结构（字段名经白名单校验、值走参数绑定），而非拼好的 SQL 字符串。各引擎模块带有 `structured_filter_security_test.go` 专项测试；若自定义引擎实现，务必保持「字段白名单 + 值参数化」两条底线。

## 5. 排序与字段掩码

- **Sorting**：`{Field, Direction}` 列表，经 `OrderByStringConverter` 接受 JSON 数组或 AIP 语法（`create_time DESC, name ASC`），产出引擎无关的排序结构。字段白名单校验与过滤层一致。
- **FieldMask**（Protobuf 原生 `fieldmaskpb.FieldMask`）：声明响应需要的字段路径，引擎据此裁剪 SELECT / 投影，既省带宽也避免敏感字段外泄。配合 [第 12 章](./12-crud-overview.md)的 DTO/Entity 分离，掩码作用点在 Entity→DTO 映射处。

## 6. 工具函数与配置

`crud/pagination` 另提供跨引擎的类型转换与过滤条件清理工具（对调用方传入的非法字段、非法操作符做修剪而非整体报错——适合「宽松接收、严格执行」的开放 API）。契约的 Protobuf 编译链（buf 安装、`buf.gen.yaml` 配置、生成命令）见 [`crud/api/README.md`](../../crud/api/README.md) 的「Protocol Buffers 编译」一节。

## 7. 深入阅读

- [`crud/api/README.md`](../../crud/api/README.md) —— 四种分页、过滤表达式、排序、字段掩码的契约全貌与完整示例（四个实战示例：简单分页 / 复杂过滤 / 令牌分页 / 日期时间过滤）
- [`crud/pagination/README.md`](../../crud/pagination/README.md) —— 转换层 API 参考（Paginator 接口、三种分页器、两类过滤器转换器、排序转换器、工具函数）、分页方式对比、操作符完整清单
- [`crud/pagination/filter/README.md`](../../crud/pagination/filter/README.md) —— JSON 与 Google AIP 两种语法的完整规范与嵌套组合解析
- [第 14 章](./14-crud-engines.md) —— 各引擎如何消费这些中间结构
