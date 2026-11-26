# users 表属于模板基础设施

**Status**: accepted

`users` 与 `refresh_tokens` 是模板内置认证纵切面的一部分，不是 `Note` 那样的可删除样例。删掉 `users` 表会同时拆掉登录、注册、`/auth/me`、`RequireAuth` 与 `seed-user`；业务项目要做的是在它上面扩展，而不是另起一套自己的用户表。边界是：加字段或关联表通过迁移、sqlc 查询、领域实体与 DTO 完成；想改认证行为时扩展 `AuthUseCase` 或替换组合根里的适配器；`users` 表本身、登录链路与 `RequireAuth` / `RequireRole` 中间件保留。`password_hash` 可空就是为外部 IdP 留的入口：**User** 可以没有本地 **Credential**。

## Consequences

- `db/*/migrations/000002_create_users.*`、`000003_create_refresh_tokens.*`、`db/*/queries/{users,refresh_tokens}.sql`、两个仓储实现与 `internal/domain/{user,principal,token}.go` 属于模板核心，不进入 README 的“可删除样例”清单。
- `Note` 仍是唯一可整体删除的示例纵切面；新增业务表应作为独立聚合加入，不修改 `users` 的语义。
