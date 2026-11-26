## Agent skills

### Issue tracker

Issues and specs for this repo live as GitHub issues (`dongyu/flash-unicorn`) via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Uses the five default canonical triage labels (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context repo (`GLOSSARY.md` and `docs/adr/` at the repo root). See `docs/agents/domain.md`.

### Quality gates

`make lint` must stay at 0 issues. Adding a linter or a `//nolint` has a required procedure, and the three gate layers (architecture boundary / correctness / style) are deliberately kept separate. See `docs/engineering/lint-policy.md` and ADR `docs/adr/0004-quality-gate-layering.md`.

### Comments

Names and types carry the meaning, so the comment layer stays empty. The only lines starting with `//`, `--`, or `#` are ones a machine reads: build tags, `//go:embed`, `//go:generate`, `//nolint:<linter>`, sqlc's `-- name:` directives, the Dockerfile `# syntax:` directive, and Makefile `## ` help text. Anything else lives in `docs/` instead — a decision goes to `docs/adr/`, an operating rule or an exemption's reason to `docs/engineering/`. Review test: delete the comment, and no reader loses anything.
