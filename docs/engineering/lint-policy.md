# Lint 门禁操作细则

配套决策见 `docs/adr/0004-quality-gate-layering.md`。本文只写"怎么做"，不重复"为什么"。

## 三层划分

| 层 | 由什么守 | 门槛 |
|---|---|---|
| 架构边界 | `depguard`（domain 纯度、usecase 只见 port） | 0 告警，不允许豁免 |
| 正确性 | `gosec`、`errcheck`、`govet`（enable-all）、`staticcheck`（checks=all 减 ST1000）、`bodyclose`、`nilerr`、`rowserrcheck`、`sqlclosecheck` 等 | 0 告警，可为修 bug 改代码 |
| 风格 / 复杂度 | `revive`、`wrapcheck`、`err113`、`mnd`、`cyclop`、`funlen`、`lll`、`dupl` | **未启用**，见 ADR 的重估条件 |

`ST1000`（包注释）被显式关掉：注释不是本仓库的信息载体，规则归属写在本文与 ADR 里。

## 新增一个 linter

1. 判断它属于哪一层。风格类不要加——先改 ADR。
2. 本地跑 `./bin/golangci-lint run --default=none --enable=<linter> ./...` 看真实告警量。
3. 正确性类：把告警**改成不告警的代码**，而不是调高阈值或加豁免。
4. 才写进 `.golangci.yml` 的 `linters.enable`。

## 豁免

```go
const fixturePostgresDSN = "postgres://yaml:yaml@localhost:5432/yaml" //nolint:gosec
```

- 必须指名 linter（`nolintlint.require-specific`），不允许裸 `//nolint`。
- 不允许留下已不再命中的死豁免（`nolintlint.allow-unused: false`）。
- **能改代码就不要豁免**：`//nolint` 是例外，不是决策工具。
- 豁免的**理由写在本节的表里**，不写在代码里。

## 当前豁免清单（改这里就要同步改本节）

| 位置 | 豁免 | 理由 |
|---|---|---|
| `internal/platform/config/config_test.go` | `gosec` G101 | 测试夹具 DSN，凭据是假的 |
| `cmd/`（`.golangci.yml` 的 `linters.exclusions.rules`） | `forbidigo` | CLI 的 stdout 就是用户界面；`internal/` 仍被强制使用 zerolog |

## 两个容易踩的坑

- **build tags**：`run.build-tags` 里包含 `integration`，否则 `integration` 标签下的文件（PostgreSQL 契约测试）对 lint 不可见，但 CI 又会跑它们。
- **生成代码**：`linters.exclusions.generated: strict` 只跳过带真实 `Code generated ... DO NOT EDIT.` 头的文件（sqlc、mockgen）。新生成器如果不写这个头，就必须被正常 lint。
