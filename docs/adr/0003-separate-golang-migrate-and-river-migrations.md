# Separate golang-migrate application migrations from River internal migrations

Status: accepted

Application tables are managed by `golang-migrate/migrate` (`db/{postgres,sqlite}/migrations`), while River's internal queue tables (`river_job`, `river_migration`, etc.) are managed exclusively by River's own `rivermigrate` mechanism and orchestrated together through a single unified migration entrypoint (`make migrate-up` / `scripts/migrate.sh`) reading the same `DATABASE_URL` config.

## Considered Options

- **Separate lifecycles, unified command entrypoint (chosen)**: Prevents conflicts when upgrading River versions while giving developers and CI/CD a single command and single DSN source.
- **Copy River SQL into `golang-migrate` files**: Breaks cleanly tracking upstream River schema updates across releases.

## Consequences

- `golang-migrate` tracks application versions in `schema_migrations`, and `rivermigrate` tracks queue versions in `river_migration`.
- Test helpers (`internal/testkit`) execute the same embedded `golang-migrate` SQL files via `source/iofs` plus `rivermigrate` so `go test ./...` requires no external CLI binary.
