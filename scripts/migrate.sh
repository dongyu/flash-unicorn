#!/usr/bin/env bash
set -euo pipefail

ACTION="${1:-up}"
shift || true

STEPS="1"
if [[ "$ACTION" == "down" && $# -gt 0 && "$1" =~ ^[0-9]+$ ]]; then
  STEPS="$1"
  shift
fi

FORCE=false
CONFIG_ARGS=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --force|-f)
      FORCE=true
      shift
      ;;
    *)
      CONFIG_ARGS+=("$1")
      shift
      ;;
  esac
done

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if [[ -n "${SERVER_BIN:-}" ]]; then
  SERVER_CMD=("$SERVER_BIN")
elif [[ -x "./bin/server" ]]; then
  SERVER_CMD=("./bin/server")
elif command -v server >/dev/null 2>&1; then
  SERVER_CMD=("server")
else
  SERVER_CMD=("go" "run" "./cmd/server")
fi

if ! command -v migrate >/dev/null 2>&1; then
  echo "error: 'migrate' CLI not found. Run 'make tools' to install it with -tags \"pgx5 sqlite\"." >&2
  exit 1
fi

MIGRATE_URL="$("${SERVER_CMD[@]}" config db-url --format=migrate ${CONFIG_ARGS[@]+"${CONFIG_ARGS[@]}"})"
RAW_URL="$("${SERVER_CMD[@]}" config db-url ${CONFIG_ARGS[@]+"${CONFIG_ARGS[@]}"})"
APP_ENV_VAL="$("${SERVER_CMD[@]}" config app-env ${CONFIG_ARGS[@]+"${CONFIG_ARGS[@]}"})"

DB_PATH=""
case "$RAW_URL" in
  postgres://*|postgresql://*)
    DIALECT="postgres"
    ;;
  sqlite://*)
    DIALECT="sqlite"
    DB_PATH="${RAW_URL#sqlite://}"
    DB_PATH="${DB_PATH%%\?*}"
    if [[ -n "$DB_PATH" && "$DB_PATH" != ":memory:" ]]; then
      mkdir -p "$(dirname "$DB_PATH")"
    fi
    ;;
  *)
    echo "error: unsupported database url scheme in '$RAW_URL'" >&2
    exit 1
    ;;
esac

MIGRATIONS_DIR="db/${DIALECT}/migrations"

run_up() {
  echo "==> Running golang-migrate ($DIALECT) up..."
  migrate -path "$MIGRATIONS_DIR" -database "$MIGRATE_URL" up
  echo "==> Running River internal migrations up..."
  "${SERVER_CMD[@]}" migrate river-up ${CONFIG_ARGS[@]+"${CONFIG_ARGS[@]}"}
}

case "$ACTION" in
  up)
    run_up
    ;;
  down)
    echo "==> Running golang-migrate ($DIALECT) down $STEPS..."
    migrate -path "$MIGRATIONS_DIR" -database "$MIGRATE_URL" down "$STEPS"
    ;;
  reset)
    if [[ "$APP_ENV_VAL" == "production" && "$FORCE" != "true" ]]; then
      echo "error: refusing to reset database when app.env=production (pass --force to override)" >&2
      exit 1
    fi
    echo "==> Resetting $DIALECT database (app + River)..."
    if [[ "$DIALECT" == "sqlite" && -n "$DB_PATH" && "$DB_PATH" != ":memory:" ]]; then
      rm -f "$DB_PATH" "${DB_PATH}-wal" "${DB_PATH}-shm"
    else
      "${SERVER_CMD[@]}" migrate river-reset ${CONFIG_ARGS[@]+"${CONFIG_ARGS[@]}"} >/dev/null 2>&1 || true
      migrate -path "$MIGRATIONS_DIR" -database "$MIGRATE_URL" drop -f
    fi
    run_up
    ;;
  status)
    echo "==> Application migration version ($DIALECT):"
    migrate -path "$MIGRATIONS_DIR" -database "$MIGRATE_URL" version || true
    echo "==> River internal migration status:"
    "${SERVER_CMD[@]}" migrate river-status ${CONFIG_ARGS[@]+"${CONFIG_ARGS[@]}"}
    ;;
  river-down)
    echo "==> Running River internal migrations down 1 step..."
    "${SERVER_CMD[@]}" migrate river-down ${CONFIG_ARGS[@]+"${CONFIG_ARGS[@]}"}
    ;;
  *)
    echo "usage: $0 {up|down [N]|reset [--force]|status|river-down} [--config path]" >&2
    exit 1
    ;;
esac
