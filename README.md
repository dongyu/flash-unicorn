# Unicorn

基于 **Clean Architecture（整洁架构）** 的可复用 Go 服务端模板，默认集成 **Go Fiber V3**、**Zerolog**、**sqlc**、**golang-migrate（CLI）**、**River（`riverqueue/river`）** 与 **Sentry**，通过单一代码库与统一配置源同时支持 **PostgreSQL** 和 **SQLite**。

## 从本模板创建新项目

1. 克隆或基于本模板新建仓库后，执行初始化脚本一键替换 Go Module 路径、架构守卫规则、应用名与环境变量前缀：

   ```bash
   ./scripts/init-project.sh github.com/your-org/your-service your-service
   # 或使用 Makefile：
   make init module=github.com/your-org/your-service name=your-service
   ```

   > 参数说明：第 1 个参数为新项目的 Go Module 路径（必填）；第 2 个参数为应用短名称（可选，未传时默认取模块路径最后一段）。

2. 安装固定版本的本地开发工具（安装到 `./bin/`，包含 `sqlc`、`mockgen`、`air`、`golangci-lint` 及带 `pgx5` + `sqlite` 驱动的 `migrate` CLI），执行初始化迁移并启动热重载：

   ```bash
   cp .env.example .env
   make tools
   make migrate-up
   make dev
   ```

3. 当你准备接入真实业务模型时，可参考示例 `Note` 链路删除或替换以下样例文件：
   - 迁移与 SQL：`db/*/migrations/000001_create_notes.*`、`db/*/queries/notes.sql`
   - 领域与用例：`internal/domain/note.go`、`internal/usecase/note.go`、`internal/usecase/note_test.go`
   - 适配器：`internal/adapter/http/dto/note.go`、`internal/adapter/http/handler/note.go`、`internal/adapter/http/handler/note_test.go`、`internal/adapter/worker/note_created.go`、`internal/adapter/repo/*/note_repo*.go`

   认证不在可删除清单里：`internal/domain/{user,principal,token}.go`、`internal/usecase/auth.go`、`internal/adapter/http/{dto,handler,middleware}/auth*.go`、`internal/platform/security/*`、`users` / `refresh_tokens` 迁移与查询都是模板基础设施，详见 [ADR-0006](./docs/adr/0006-users-table-is-template-infrastructure.md)。

---

## 内置认证

模板已内置一条可直接使用的认证纵切面：**User** 落库，密码用 bcrypt 存储，登录签发 **AccessToken**（HS256 JWT，默认 15 分钟）与 **RefreshToken**（不透明、只能存哈希、可轮换、可吊销）。`notes` 示例路由全部要求 **AccessToken**，作为 `RequireAuth` 的用法示范。

| 方法 | 路径 | 说明 |
|---|---|---|
| `POST` | `/api/v1/auth/register` | 注册并直接返回令牌对；生产环境默认不注册该路由 |
| `POST` | `/api/v1/auth/login` | 邮箱 + 密码换取令牌对 |
| `POST` | `/api/v1/auth/refresh` | 用 **RefreshToken** 换取新令牌对，旧令牌立即失效 |
| `POST` | `/api/v1/auth/logout` | 吊销当前 **RefreshToken**；`all=true` 时吊销该 **User** 的全部会话 |
| `GET` | `/api/v1/auth/me` | 需要 **AccessToken**，回库读取当前 **User** 的邮箱、显示名、**Role** 与状态 |

错误统一走 RFC 9457 ProblemDetails；`401` 附带 `WWW-Authenticate: Bearer`，登录与注册接口按 IP 限流（`429`）。

### 创建第一个用户

先执行迁移，再用 `seed-user` 在不开 HTTP 的情况下创建管理员：

```bash
make migrate-up
make seed-user email=admin@example.com role=admin
```

命令会交互式读取密码；密码只经 stdin 传给进程，不接受 `password=` 这类 make 变量，因此不会留在 shell 历史或进程参数里。开发环境也可以直接调注册接口：

```bash
curl -sS -X POST http://localhost:3331/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@example.com","password":"change-me-please"}'
```

### 带令牌调用

登录或注册的响应形如 `{"access_token":"...","refresh_token":"...","token_type":"Bearer","expires_in":900,"user":{...}}`。后续请求把 **AccessToken** 放进 `Authorization` 头：

```bash
TOKEN=$(curl -sS -X POST http://localhost:3331/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@example.com","password":"change-me-please"}' \
  | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')

curl -sS -H "Authorization: Bearer $TOKEN" http://localhost:3331/api/v1/auth/me
curl -sS -H "Authorization: Bearer $TOKEN" http://localhost:3331/api/v1/notes
```

加用户字段、换外部 IdP、加细粒度授权分别要改哪里，见 [docs/engineering/auth.md](./docs/engineering/auth.md)；令牌模型与 `users` 表的边界见 [ADR-0005](./docs/adr/0005-access-token-and-refresh-token.md) 与 [ADR-0006](./docs/adr/0006-users-table-is-template-infrastructure.md)。

---

## 架构设计总览

依赖方向严格向内收敛至 `internal/domain`，由 [.golangci.yml](./.golangci.yml) 中的 `depguard` 规则在编译/CI 阶段强制校验：

```text
cmd/server ─▶ internal/app ─▶ internal/adapter/{http,worker,repo,queue} ─▶ internal/usecase ─▶ internal/domain
                   └──────────────▶ internal/platform/{buildinfo,config,logger,database,queue,telemetry}
```

- **架构决策记录（ADR）**：关键技术权衡（仅支持 PostgreSQL + SQLite、双库 `sqlc` + `context` 透传事务、分离 `golang-migrate` 与 `rivermigrate` 生命周期、令牌模型、`users` 表属于模板核心设施）详见 [docs/adr/](./docs/adr/)。
- **模块演进规范**：业务实体较少时在 `internal/domain` 与 `internal/usecase` 内按文件平铺；当核心聚合超过 4~5 个时，建议在各层内按业务域建对齐的子包（例如 `domain/order`、`usecase/order`、`adapter/http/handler/order`）。

---

## 目录结构

```text
├── cmd/server/                  # 单一二进制入口 (all | api | worker | version | config | migrate | seed-user)
├── configs/                     # 可选 YAML 配置示例 (config.example.yaml)
├── db/
│   ├── embed.go                 # 内嵌业务迁移 SQL，供 testkit 零依赖执行
│   ├── postgres/                # PostgreSQL 迁移文件 (migrations/) 与 sqlc 查询 (queries/)
│   └── sqlite/                  # SQLite 迁移文件 (migrations/) 与 sqlc 查询 (queries/)
├── docs/
│   ├── adr/                     # 架构决策记录 (ADR)
│   └── engineering/             # 工程操作手册 (lint、认证等)
├── internal/
│   ├── domain/                  # 纯业务实体、哨兵错误、Clock、IDGenerator、Repository/Queue/Tx 接口
│   ├── usecase/                 # 应用用例编排（零外部框架依赖，使用 zerolog.Ctx 打日志）
│   ├── adapter/
│   │   ├── http/
│   │   │   ├── router.go        # Fiber V3 总路由与中间件装配 (基于 []RouteRegistrar 解耦 UseCase)
│   │   │   ├── dto/             # 请求/响应 DTO、分页结构与 RFC 9457 ProblemDetails
│   │   │   ├── middleware/      # 统一错误映射、请求上下文日志注入与 Sentry Tag
│   │   │   └── handler/         # HTTP Handler、泛型参数绑定校验 (BindAndValidate)、/healthz 与 /readyz
│   │   ├── worker/              # River 异步任务参数与 Worker 实现
│   │   └── repo/
│   │       ├── dberr/           # 数据库唯一约束等错误归一化 (-> domain.ErrConflict)
│   │       ├── postgres/        # sqlc (pgx/v5) 生成代码 + Repository 与 TxManager 实现
│   │       ├── sqlite/          # sqlc (modernc.org/sqlite) 生成代码 + Repository 与 TxManager 实现
│   │       ├── queue/           # River 入队适配器（自动参与当前事务）
│   │       └── txctx/           # 通过 context.Context 透传 pgx.Tx / *sql.Tx
│   ├── platform/                # buildinfo、koanf 配置、Zerolog、数据库连接池、River 客户端、Sentry 与安全适配器
│   ├── bootstrap/               # 双库仓储装配（服务与命令行共用，方言分派只此一处）
│   ├── app/                     # 手写依赖组装 (Composition Root) 与优雅停机编排
│   ├── seed/                    # 不依赖 HTTP 的建用户入口（server seed-user 使用）
│   └── testkit/                 # 零 Docker SQLite 测试套件、HTTP/Problem 断言助手、双库契约测试基类
│       └── mocks/               # GoMock 生成桩，按来源包拆分为 domain/ 与 usecase/ 避免循环依赖
├── scripts/
│   ├── init-project.sh          # 新项目初始化脚本
│   └── migrate.sh               # 统一迁移脚本 (golang-migrate CLI + River)
├── sqlc.yaml                    # 双库 sqlc 配置（直读 migrations/ 并统一映射 uuid.UUID / time.Time）
├── Makefile                     # 常用开发、生成、迁移、Lint 与测试命令
├── Dockerfile                   # 多阶段 CGO_ENABLED=0 构建 (Debian bookworm-slim 运行时)
├── compose.yaml                 # Postgres + migrate + api + worker 编排示例
└── .github/workflows/ci.yml     # GitHub Actions CI（迁移检查、Lint、SQLite/Postgres 测试与 Docker 构建）
```

---

## 新增业务功能开发清单

以新增一个 `Order` 模块为例，标准开发链路如下：

1. **数据库迁移**：执行 `make migrate-create name=create_orders`，分别完善 `db/postgres/migrations/` 与 `db/sqlite/migrations/` 的 DDL，然后运行 `make db-check` 校验双库迁移文件对齐。
2. **编写 SQL 与生成代码**：在 `db/postgres/queries/` 与 `db/sqlite/queries/` 编写查询，运行 `make gen` 同步执行 `sqlc generate` 与 GoMock 生成。
3. **定义领域与用例**：在 `internal/domain/` 定义实体与端口接口（添加 `//go:generate mockgen ...` 指向 `internal/testkit/mocks/domain/`），在 `internal/usecase/` 编排业务逻辑并用 `TxManager.WithinTx` 保证写库与发队列原子性。
4. **实现适配器与组装**：在 `internal/adapter/repo/{postgres,sqlite}/` 实现仓储适配器，在 `internal/adapter/http/{dto,handler}/` 添加 DTO 与 Handler，在 [internal/app/app.go](./internal/app/app.go) 完成组装并把 Handler 注入 `Routes` 切片（无需改动 [router.go](./internal/adapter/http/router.go)）。受保护路由用 `middleware.RequireAuth`（见 `internal/adapter/http/handler/note.go`），管理接口再叠加 `middleware.RequireRole(domain.RoleAdmin)`。
5. **补齐三层测试**：
   - **单元测试**：`internal/usecase/*_test.go`、`internal/adapter/http/handler/*_test.go`（使用 GoMock + `testkit.FakeClock` + `testify/require`）。
   - **双库契约测试**：在 `internal/testkit/` 编写 `testify/suite` 契约套件，默认随 SQLite 测试运行，CI 通过 `make test-int` 在 PostgreSQL 上跑同一套断言。
   - **全链路集成测试**：使用 `testkit.NewTestApp(t)`、`testkit.DoJSON`、`rivertest.RequireInserted` 与 `testkit.WorkSQLiteJob` 验证 HTTP → SQLite → River 全链路。

---

## 统一配置与数据库迁移

所有组件（**Server**、**`migrate` CLI**、**River** 及可选 **`sqlc vet`**）共享同一配置源 `database.url` / `DATABASE_URL`，由 `koanf` 按 **`默认配置 < 可选 YAML < .env < 环境变量`** 优先级解析，并根据连接串前缀（`postgres://` / `postgresql://` 或 `sqlite://`）自动识别数据库方言：

- **`sqlc`**：在 [sqlc.yaml](./sqlc.yaml) 中直接指向 `db/{postgres,sqlite}/migrations` 作为 Schema 来源（自动忽略 `.down.sql`），消除重复维护。
- **统一迁移命令**（底层业务表由 `golang-migrate` CLI 管理，River 表由 `rivermigrate` 管理）：

  ```bash
  make migrate-up      # 执行业务表迁移 + River 内部表迁移
  make migrate-down    # 回滚 1 步业务表迁移
  make migrate-reset   # 开发期清库重建（生产环境默认拦截，需显式传 --force）
  make migrate-status  # 查看业务表与 River 当前迁移版本
  ```

- **Sentry 可观测性**：通过 `SENTRY_DSN`（或 YAML `sentry.dsn`）启用；本地开发与测试留空即可自动禁用，生产环境会自动附带 `request_id`、`job_id`/`job_kind` 标签及构建版本 `Release`。

---

## 常用命令与 Docker 部署

| 命令 | 说明 |
|---|---|
| `make tools` | 安装固定版本的 `sqlc`、`mockgen`、`air`、`golangci-lint` 与 `migrate` 到 `./bin/` |
| `make gen` | 执行 `sqlc generate` 与 GoMock 生成 |
| `make dev` | 启动 Air 热重载开发环境 |
| `make seed-user` | 创建用户（`email=...`，可选 `role=admin`），密码从 stdin 读取 |
| `make test` | 运行全量单元测试与零 Docker SQLite 集成测试 |
| `make test-int` | 运行含 PostgreSQL 契约测试在内的完整测试（需设置 `TEST_POSTGRES_DSN`） |
| `make lint` | 运行 `golangci-lint`（含 Clean Architecture `depguard` 依赖边界检查与 `integration` build tag 代码） |
| `make vuln` | 用 `govulncheck` 扫描依赖中的已知漏洞 |
| `make tidy` | 校验 `go.mod` / `go.sum` 无需 tidy |
| `make build` | 编译纯静态二进制到 `./bin/server`（注入版本元数据） |

使用 Docker Compose 一键启动 `PostgreSQL + 自动迁移 + API + Worker` 全栈：

```bash
docker compose up --build
```
