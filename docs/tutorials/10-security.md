# Go-Wind 插件教程 · 第 10 章：认证、授权与加密（Security）

> 本篇是 [Go-Wind 插件系列教程](./README.md)的组成部分。安全家族分三个子域：认证（authn，你是谁）、授权（authz，你能做什么）、加密（crypto，传输与载荷保密）。

## 1. 家族结构

认证子域的引擎模块（`security/authn/` 下）：`apikey`、`basicauth`、`hmac`、`jwt`、`mtls`、`oauth2`、`oidc`、`presharedkey`、`session`。
授权子域的引擎模块（`security/authz/` 下）：`acl`、`awsiam`、`casbin`、`cedar`、`cerbos`、`opa`、`rbac`、`zanzibar`。
加密子域：`security/crypto`（信封加密等通用密码学工具，底层来自 go-utils/crypto）。

各引擎的集成面是统一的：服务侧通过传输层中间件（`transport/http/middleware/authn` / `authz`、gRPC 同名套件，见[第 8 章](./08-transport-middleware.md)）挂到请求链上——认证中间件解析凭据并把身份放进 context，授权中间件拿身份做策略判定，业务 handler 里的身份信息始终来自 context 而非请求参数。HTTP 侧中间件的挂载方式在[HTTP 实战教程 §7.8](../http-server-tutorial.md)有逐行示例。

## 2. 引擎概念速览

**认证（authn）**：

- [`jwt`](../../security/authn/jwt/README.md) —— JSON Web Token：签名令牌的无状态校验。适用：自家签发的 API 会话。注意令牌无法主动吊销，短有效期 + 刷新令牌是标配。
- [`oidc`](../../security/authn/oidc/README.md) —— OpenID Connect：委托外部 IdP 完成认证（Discovery 端点、JWKS 公钥验签）。适用：接入企业 SSO / 云身份。
- [`oauth2`](../../security/authn/oauth2/) —— OAuth2 客户端凭据与令牌流程。
- [`presharedkey`](../../security/authn/presharedkey/README.md) —— 预共享密钥：双方约定的静态密钥比对。适用：服务间内部调用的最低成本方案，密钥轮转纪律必须严格。
- [`apikey`](../../security/authn/apikey/) —— API Key 比对。
- [`basicauth`](../../security/authn/basicauth/) —— HTTP Basic 认证头比对。
- [`hmac`](../../security/authn/hmac/) —— HMAC 签名校验（时间戳+密钥防重放）。
- [`mtls`](../../security/authn/mtls/) —— 双向 TLS：以客户端证书为身份。适用：零信任内网、服务网格外的补充手段。
- [`session`](../../security/authn/session/) —— 服务端会话（cookie/session store）。

**授权（authz）**：四种模型范式：

- ACL / RBAC（[`acl`](../../security/authz/acl/)、[`rbac`](../../security/authz/rbac/)）—— 主体-资源-动作的静态权限表 / 角色层级。
- 策略引擎（[`opa`](../../security/authz/opa/README.md) —— Rego 策略语言，策略可经 `policy.bindata` 嵌入二进制；[`cedar`](../../security/authz/cedar/)、[`cerbos`](../../security/authz/cerbos/)）—— 授权逻辑与代码解耦，策略独立版本化。
- 关系型访问控制 ReBAC（[`zanzibar`](../../security/authz/zanzibar/README.md) —— Google Zanzibar 论文的开源实现族，概念为关系元组（Relation Tuples）与命名空间/对象/主体；本仓对接 [`keto`](../../security/authz/zanzibar/keto/README.md)、[`openfga`](../../security/authz/zanzibar/openfga/README.md)）—— 按「主体与对象的关系」而非静态角色判定，适合组织架构、文档归属这类继承性权限。
- 云 IAM（[`awsiam`](../../security/authz/awsiam/)）—— 委托 AWS IAM 策略判定。

## 3. 选型路径

| 需求 | 推荐组合 |
|------|----------|
| 对外 API，自家用户体系 | `authn/jwt`（短期令牌）+ `authz/rbac` 或 `authz/casbin` |
| 企业 SSO 接入 | `authn/oidc` + 授权按 IdP 冗来的组/角色映射到 `authz/rbac` |
| 微服务间调用 | `authn/mtls` 或 `authn/hmac` + 服务级 ACL；或上服务网格 |
| 权限随组织结构继承（部门可见、文档归属） | `authz/zanzibar` 系（ReBAC） |
| 授权规则需要独立审计与热更新 | `authz/opa` / `authz/cerbos`（策略即代码） |

## 4. 不可妥协的原则

- **默认拒绝**：中间件链上授权判定失败 = 403，没有「忘了配就是放行」的路径。
- **身份只认 context**：handler 里的角色/用户信息从认证中间件注入的 context 取，绝不信请求体里的自报身份。
- **密钥生命周期**：PSK/JWT 密钥必须有轮转机制；JWT 校验只信白名单 issuer/audience 与 JWKS 端点。
- **加密≠认证**：`crypto` 模块解决的是载荷保密（信封加密等），身份判定仍要 authn/authz 各司其职，不要拿「能解密」当「已认证」。

## 5. 深入阅读

模块 README：[`jwt`](../../security/authn/jwt/README.md) · [`oidc`](../../security/authn/oidc/README.md) · [`presharedkey`](../../security/authn/presharedkey/README.md) · [`casbin`](../../security/authz/casbin/README.md) · [`opa`](../../security/authz/opa/README.md) · [`zanzibar`](../../security/authz/zanzibar/README.md) · [`zanzibar/keto`](../../security/authz/zanzibar/keto/README.md) · [`zanzibar/openfga`](../../security/authz/zanzibar/openfga/README.md)

其余引擎见源码目录 [`security/`](../../security/)。另见 [根 README · 安全矩阵](../../README.md#安全security)。
