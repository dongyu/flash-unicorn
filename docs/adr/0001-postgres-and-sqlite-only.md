# Persist business data and River queues on PostgreSQL and SQLite only

Status: accepted

We restrict database support to PostgreSQL and SQLite (shared by both business persistence and the River job queue), explicitly excluding MySQL and other dialects. `riverqueue/river` only supports PostgreSQL and SQLite (`riversqlite`), and `sqlc` requires separate schema and query code per SQL dialect without cross-engine translation.

## Considered Options

- **PostgreSQL + SQLite unified (chosen)**: Business data and River share the configured engine (SQLite for lightweight/local single-node setups, PostgreSQL for production).
- **Multi-engine business DB + separate River DB**: Supporting MySQL for business tables would force operating a separate PostgreSQL/SQLite instance solely for River and maintaining three+ `sqlc` dialect stacks.
- **Swap River for a Redis-backed queue**: Would allow MySQL, at the cost of losing transactional job enqueuing inside the same SQL transaction.

## Consequences

- Every repository interface in `domain` has at most two `sqlc`-backed implementations (`postgres` and `sqlite`).
- Adding MySQL or another SQL dialect later will require either introducing a third `sqlc` target plus a separate queue backend or replacing River.
