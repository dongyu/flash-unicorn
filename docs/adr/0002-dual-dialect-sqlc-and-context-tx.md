# Use dialect-split sqlc packages with context-propagated transactions

Status: accepted

We maintain separate `sqlc` schemas, queries, and generated packages for PostgreSQL (`pgx/v5`) and SQLite (`modernc.org/sqlite`, `MaxOpenConns(1)`), mapped manually into `domain` repository interfaces. Database transactions are managed through a `TxManager.WithinTx` abstraction that propagates the engine-specific transaction (`pgx.Tx` or `*sql.Tx`) via `context.Context` so repositories and River `InsertTx` share one atomic transaction without leaking driver types into `usecase`.

## Considered Options

- **Dialect-split `sqlc` + manual adapters + context tx (chosen)**: Keeps generated code predictable, avoids CGO via `modernc.org/sqlite`, and hides driver transaction types from business logic.
- **`sqlc-multi-db` code generation wrapper**: Reduces adapter boilerplate at the cost of extra toolchain coupling and less control over dialect-specific SQL differences.
- **ORM (GORM / Ent) or explicit tx parameters**: An ORM sacrifices SQL-first control; passing driver tx values explicitly leaks infrastructure types into `usecase`.

## Consequences

- Every new query must be written and verified in both `postgres` and `sqlite` SQL dialects.
- SQLite runs with a single writer connection (`MaxOpenConns(1)`), `WAL`, and `busy_timeout` to prevent `SQLITE_BUSY` with River; nested `WithinTx` calls must reuse the transaction already present in `ctx`.
