# Go-Wind 插件教程 · 第 17 章：对象存储（OSS）

> 本篇是 [Go-Wind 插件系列教程](./README.md)的组成部分。对象存储家族有两个模块，各自定义本地 `Config`——两个实现的底层 SDK 与返回类型不兼容，本层不提取共享接口。

## 1. 插件矩阵

| 插件 | 模块路径 | 引擎 |
|------|---------|------|
| MinIO | `github.com/tx7do/go-wind-plugins/oss/minio` | minio/minio-go |
| S3 | `github.com/tx7do/go-wind-plugins/oss/s3` | aws/aws-sdk-go-v2 |

## 2. 两个模块的分工

- **[MinIO](../../oss/minio/README.md)**：对接自建的 S3 兼容对象存储（MinIO 部署形态）。文档结构：概述 → 安装 → Config（端点与凭据配置）→ API → 使用示例（创建客户端、上传与下载）。
- **[S3](../../oss/s3/README.md)**：对接 AWS S3 原生服务，亦覆盖以 S3 兼容端点运行的本地服务（文档专门有「AWS S3」与「MinIO / 本地兼容服务」两个接入示例章节）。

两者面向同一业务动作（对象的上传、下载、预签名 URL 等），但客户端类型不互通——迁移目标存储时，调用点需要按模块 README 的示例逐处调整，这是本层不做统一接口的直接后果（换接口会丢失两家 SDK 各自的能力面）。

## 3. 接入要点

- **端点与凭据**走各模块 `Config` 结构，凭据来源遵循[第 1 章](./01-config.md)的原则：从配置中心（含 `config/vault`）注入，不落代码库。
- **对象存储不是数据库**：大文件、制品、备份归这里；带查询语义的数据走[第 12 章](./12-crud-overview.md)的存储引擎。
- **预签名 URL 优先**：给客户端的下载能力尽量用短时效预签名 URL，而不是经应用服务器中转流量。
- 与 `config/oss` 插件（[第 1 章](./01-config.md)）的关系：那是把对象存储当作**配置源**（配置内容存对象）；本章是把对象存储当作**业务存储**。

## 4. 深入阅读

- [`oss/minio/README.md`](../../oss/minio/README.md) · [`oss/s3/README.md`](../../oss/s3/README.md)
- [根 README · 对象存储矩阵](../../README.md#对象存储)
