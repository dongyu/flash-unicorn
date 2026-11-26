# 访问令牌无状态、刷新令牌可轮换

**Status**: accepted

认证只对外发放两种令牌。**AccessToken** 是 HS256 签名的 JWT，默认 15 分钟有效，claims 固定为 `sub`（User 的 UUID）、`iss`、`iat`、`exp`、`jti` 与 `roles`（数组，当前每个 User 只有一个 **Role**）；校验时只接受 HS256，强制要求 `exp` 与 `iss` 存在，并允许 30 秒时钟偏移。**RefreshToken** 是 32 字节加密随机数的 base64url 编码，不透明、不是 JWT，默认 720 小时有效，数据库只保存其 SHA-256；每次刷新都轮换，旧记录置 `revoked_at`、新记录用 `rotated_from_id` 指回旧记录。若提交的 **RefreshToken** 已存在但已被吊销，视为重放：吊销该 **User** 名下全部未吊销的 **RefreshToken**，并返回 `401`。

## Consequences

- **AccessToken** 无状态，服务端不查库也不维护拒绝名单，因此禁用 **User** 或修改 **Role** 最多要等一个 **AccessToken** TTL（默认 15 分钟）才影响已签发的令牌；`jti` 只是为将来预留的标识，不构成吊销机制。
- HS256 意味着验签方必须持有同一把签名密钥。单服务部署没有问题；多服务验签需要共享 `auth.secret`（通过密钥管理下发），或者替换 `TokenIssuer` 适配器改用非对称签名或外部 IdP。调用 `RequireAuth`、`AuthUseCase` 与 `Principal` 的代码不需要跟着改。
