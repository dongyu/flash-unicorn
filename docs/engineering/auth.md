# 认证操作手册

配套决策见 `docs/adr/0005-access-token-and-refresh-token.md` 与 `docs/adr/0006-users-table-is-template-infrastructure.md`。本文只写“怎么改”和“上线前检查什么”，不重复“为什么”。

## 三条扩展路径

### 1. 给 User 加一个字段

按数据流从外到内改，只碰下面这些面：

| 层 | 文件 | 要做什么 |
|---|---|---|
| 迁移 | `db/postgres/migrations/` 与 `db/sqlite/migrations/` 各新增一对 `.up.sql` / `.down.sql` | 两边文件名保持一致，`make db-check` 会校验；已有表用 `ALTER TABLE` |
| 查询 | `db/{postgres,sqlite}/queries/users.sql` | `INSERT` / `SELECT` 跟着列走，保持双方言 SQL 语义一致 |
| 生成 | `make gen` | 重新生成 `internal/adapter/repo/{postgres,sqlite}/gen/` |
| 领域 | `internal/domain/user.go` | 在 `User` 上加字段；需要归一化就在这里加函数 |
| 仓储 | `internal/adapter/repo/{postgres,sqlite}/user_repo.go` | 把生成模型映射回 `domain.User` |
| DTO | `internal/adapter/http/dto/auth.go` | 在 `AuthUserResponse` 与 `ToAuthUserResponse` 中暴露字段 |
| 用例 | `internal/usecase/auth.go` | 只在注册或种子创建需要接收该字段时，扩展 `RegisterInput` / `SeedUserInput` |
| 种子 | `internal/app/seed.go`、`internal/usecase/seed_user.go`、`internal/testkit/auth.go` | 需要从 `make seed-user` 或测试夹具写入时同步 |
| 处理器 | `internal/adapter/http/handler/auth.go` | 只是把 DTO 映射到新的用例输入，不承载业务判断 |

令牌与中间件不需要动：**AccessToken** 只携带 `sub` 与 `roles`，新增字段通过 `GET /api/v1/auth/me` 回库读取。

### 2. 换成外部 IdP

认证有两个可替换的适配器，位置都在 `internal/platform/security/`，装配都在 `internal/app/app.go`：

- `domain.TokenIssuer`（`IssueAccessToken` / `VerifyAccessToken`）：换掉 `security.NewJWTIssuer(...)`。外部 IdP 签发的令牌可以在新适配器里用 JWKS 验签，再把 claims 映射成 `domain.Principal{UserID, Roles}`；调用 `middleware.RequireAuth(issuer)` 的代码与 `AuthUseCase` 一行都不用改。
- `domain.PasswordHasher` 与 `domain.RefreshTokenGenerator`：如果 IdP 接管了密码校验或刷新，替换这两个实现即可；登录/刷新接口的 DTO 与 `AuthHandler` 仍然复用。

替换 `TokenIssuer` 时保持三个约束：

1. `VerifyAccessToken` 必须返回 `domain.Principal`，并在失败时包装 `domain.ErrUnauthorized`，否则 401 映射会退化成 500。
2. 算法白名单、`exp` 与 `iss` 的校验必须在新适配器里显式打开，不要依赖库的默认值。
3. 领域层与用例层不得 import JWT/JWKS 库；`.golangci.yml` 的 `depguard` 会拒绝。

如果外部 IdP 同时接管登录与刷新，替换边界上移到 `AuthUseCase` / `AuthHandler`；`RequireAuth`、`RequireRole`、`PrincipalFromContext` 与受保护路由保持不变。

### 3. 在 RequireRole 之上加细粒度授权

- 路由级角色：在 `internal/app/app.go` 装配处叠加中间件，或在一个 `RouteRegistrar` 的路由组上挂载：

  ```go
  admin := api.Group("/admin", middleware.RequireAuth(issuer), middleware.RequireRole(domain.RoleAdmin))
  ```

- 资源归属：在 `internal/usecase/` 里用 `principal, ok := domain.PrincipalFromContext(ctx)` 取当前 **Principal**，比较 `principal.UserID` 与资源所有者；不要把身份参数穿进每个函数签名。
- 更复杂的策略：再加一个中间件或用例内判断，失败时返回 `domain.ErrForbidden`。当前刻意不引入 `Authorizer` 端口：只有一个实现、一个调用点的接口不是扩展点；出现第二个策略、第二个调用方时再抽。

## 运维口径

- **更换 `AUTH_SECRET`**：所有已签发的 **AccessToken** 会立刻验签失败（客户端收到 `401`）。**RefreshToken** 是不透明值，不受签名密钥影响；要强制所有会话下线，还需要吊销 `refresh_tokens` 中的记录。
- **限流**：`middleware.NewAuthRateLimiter` 是进程内内存限流（默认每个 IP 每分钟 10 次），单实例有效。多副本部署时每个副本各自计数，必须换成共享存储（如 Redis）或在网关层限流；模板不内置共享限流存储。
- **注册开关**：`POST /api/v1/auth/register` 只在 `auth.allow_registration` 为真时注册路由。非 `production` 环境默认开放，`production` 默认关闭；要让生产开放，必须显式设置 `auth.allow_registration: true` 或 `AUTH_ALLOW_REGISTRATION=true`。
- **生产密码成本**：`production` 下 `auth.bcrypt_cost` 必须不小于 10，否则配置校验直接失败；开发与测试默认 4 只是为了不让测试为“故意变慢”付几十秒。成本上限由 `auth.bcrypt_cost` 的校验区间 `[4,14]` 兜住。
- **启动失败项**：`auth.secret` 为空、长度小于 32、或在非 `development` 环境仍等于开发默认值，都会在启动阶段失败。

## 认证日志白名单

认证路径的日志只允许出现这些字段：

| 字段 | 来源 | 用途 |
|---|---|---|
| `user_id` | 已定位到 **User** 时由用例写入 | 关联同一次失败或成功；未知用户不写 |
| `reason` | 用例写入 | `unknown_user`、`missing_credential`、`password_mismatch`、`disabled_user`、`unknown_refresh_token`、`revoked_refresh_token`、`expired_refresh_token`、`concurrent_rotation` |
| `request_id` | `requestid` 中间件 | 串联同一次请求的访问日志与用例日志 |
| 路由 / 方法 / 状态码 | 访问日志中间件 | 定位端点，不携带请求体 |

`level`、`time`、`message` 是 Zerolog 的通用信封字段，不属于业务信息。

禁止把邮箱、密码、**AccessToken**、**RefreshToken**、`Authorization` 头或请求体写进日志，原因有三条：

1. 邮箱是 PII，且能把“登录失败”还原成“账号是否存在”，日志读取权限通常远宽于用户管理权限。
2. 密码与 **RefreshToken** 本身就是凭据，一旦落盘就等同于泄露；**AccessToken** 是 Bearer 凭据，拿到即可在有效期内调用接口。
3. 日志会进入聚合、轮转、第三方采集与告警链路，删除成本远高于写入成本，不能靠“以后清理”。

客户端统一只看到 `401 invalid credentials` 或 `401 invalid refresh token`；服务端用 `reason` 区分失败原因。

## auth.* 配置

| YAML 键 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| `auth.issuer` | `AUTH_ISSUER` | 应用名（模板为 `unicorn`） | JWT 的 `iss`，校验时强制匹配 |
| `auth.secret` | `AUTH_SECRET` | 开发默认签名密钥 | HS256 签名与验签，长度至少 32；非 `development` 不得沿用默认值 |
| `auth.access_ttl` | `AUTH_ACCESS_TTL` | `15m` | **AccessToken** 有效期，也是禁用/改角色生效延迟的上限 |
| `auth.refresh_ttl` | `AUTH_REFRESH_TTL` | `720h` | **RefreshToken** 有效期 |
| `auth.leeway` | `AUTH_LEEWAY` | `30s` | JWT 校验允许的时钟偏移 |
| `auth.bcrypt_cost` | `AUTH_BCRYPT_COST` | `4`（生产示例 `12`） | bcrypt 成本，区间 `[4,14]`，`production` 下限为 `10` |
| `auth.allow_registration` | `AUTH_ALLOW_REGISTRATION` | 非 `production` 为 `true`，`production` 为 `false` | 为真时才注册 `POST /api/v1/auth/register` |

对应的开发默认值与生产示例分别在 `.env.example` 与 `configs/config.example.yaml`。
