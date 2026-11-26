# 质量门禁分层

`.golangci.yml` 同时承担了架构边界、正确性、风格三类检查。我们刻意只把前两类做成硬门禁（0 告警），
第三类暂缓启用，理由是：模板仓库初期就上满风格规则，会逼出大量 `//nolint`，而豁免遍地本身就击穿了"下限"。

**Status**: accepted

## Considered Options

- **一次拉满（revive / wrapcheck / err113 / mnd / cyclop / funlen / lll / dupl 全开）**：当时实测需要改动或豁免 150+ 处，
  其中 `wrapcheck` 50 处、`err113` 20 处与仓库刻意"不包装领域错误"的设计直接冲突。否决。
- **什么都不加**：`gosec` 当时已能指出 `int→int32` 分页溢出与 `0777` 目录权限这类真实缺陷，否决。
- **分层（采纳）**：正确性规则零告警，风格规则延后并留下重新评估的触发条件。

## Consequences

- 新增 linter 的默认口径：**正确性类必须 0 告警**（允许为此改代码），**风格类必须 0 改动**才能进（否则归入第三层）。
- `//nolint` 本身由 `nolintlint` 把关：必须指名 linter，不允许留成死豁免。
- 代码里不写解释性注释：规则归属看 ADR，豁免理由看 `docs/engineering/lint-policy.md`。
- 重新评估第三层的触发条件：测试文件数 ≥ 15，或非测试代码 ≥ 8000 行。届时先定错误分层策略（哪些错误该 wrap、
  哪些是哨兵错误），再谈 `wrapcheck` / `err113`。
- 操作细则与当前豁免清单见 `docs/engineering/lint-policy.md`。
