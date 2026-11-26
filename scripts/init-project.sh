#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <new-go-module> [new-app-name]" >&2
  echo "example: $0 github.com/acme/orders-api orders" >&2
  exit 1
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "error: python3 is required to run init-project.sh" >&2
  exit 1
fi

NEW_MODULE="$1"
DEFAULT_APP_NAME="$(basename "$NEW_MODULE" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9_-]/-/g')"
NEW_APP="${2:-$DEFAULT_APP_NAME}"
NEW_ENV_PREFIX="$(printf '%s' "$NEW_APP" | tr '[:lower:]-' '[:upper:]_' | sed 's/[^A-Z0-9_]/_/g')_"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

OLD_MODULE="$(awk '/^module /{print $2; exit}' go.mod)"
if [[ -z "$OLD_MODULE" ]]; then
  echo "error: could not detect current module name in go.mod" >&2
  exit 1
fi

echo "==> Initializing template: module '$OLD_MODULE' -> '$NEW_MODULE', app '$NEW_APP', env prefix '$NEW_ENV_PREFIX'"

while IFS= read -r -d '' file; do
  python3 - "$file" "$OLD_MODULE" "$NEW_MODULE" "$NEW_APP" "$NEW_ENV_PREFIX" <<'PY'
import pathlib, sys

path = pathlib.Path(sys.argv[1])
old_mod, new_mod, new_app, new_prefix = sys.argv[2], sys.argv[3], sys.argv[4], sys.argv[5]
text = path.read_text(encoding="utf-8")

updated = text.replace(f"{old_mod}/", f"{new_mod}/")
if path.name == "go.mod":
    updated = updated.replace(f"module {old_mod}", f"module {new_mod}", 1)

if old_mod == "unicorn":
    if path.name == "config.go":
        updated = updated.replace('"UNICORN_"', f'"{new_prefix}"')
    elif path.name == "router.go":
        updated = updated.replace('AppName:      "unicorn"', f'AppName:      "{new_app}"')
    elif path.name in ("compose.yaml", "config.example.yaml", ".env.example", "Makefile", "ci.yml", "README.md"):
        updated = updated.replace("unicorn", new_app)

if updated != text:
    path.write_text(updated, encoding="utf-8")
PY
done < <(find . -maxdepth 6 \( -name '*.go' -o -name 'go.mod' -o -name '.golangci.yml' -o -name 'compose.yaml' -o -name 'config.example.yaml' -o -name '.env.example' -o -name 'Makefile' -o -name 'Dockerfile' -o -name 'ci.yml' -o -name 'README.md' \) -not -path './.git/*' -print0)

echo "==> Done. Next steps:"
echo "  1. Remove or replace the sample 'Note' domain/usecase/handler/migrations if starting a fresh domain."
echo "  2. Run 'go test ./...' to verify the renamed module compiles and passes."
