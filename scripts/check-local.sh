#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ "${GOTOOLCHAIN:-}" == "" || "${GOTOOLCHAIN:-}" == "local" ]]; then
  export GOTOOLCHAIN="auto"
fi
export GOCACHE="${GOCACHE:-/tmp/mimir-go-build-cache}"
export GOMODCACHE="${GOMODCACHE:-/tmp/mimir-go-mod-cache}"

cd "$PROJECT_ROOT"

# go test ./... covers every Go package and is portable (no GNU mapfile / find -printf).
# Build the frontend first so go:embed (all:frontend/dist) resolves during go test.
echo "== Frontend build =="
(
  cd frontend
  npm run build
)

echo "== Go tests =="
go test ./...

echo "== Local check complete =="
