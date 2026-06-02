#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ "${GOTOOLCHAIN:-}" == "" || "${GOTOOLCHAIN:-}" == "local" ]]; then
  export GOTOOLCHAIN="auto"
fi
export GOCACHE="${GOCACHE:-/tmp/mimir-go-build-cache}"
export GOMODCACHE="${GOMODCACHE:-/tmp/mimir-go-mod-cache}"

cd "$PROJECT_ROOT"

mapfile -t GO_PACKAGES < <(find . -path './frontend' -prune -o -path './build' -prune -o -name '*.go' -printf '%h\n' | sort -u)

echo "== Go tests =="
go test "${GO_PACKAGES[@]}"

echo "== Frontend build =="
(
  cd frontend
  npm run build
)

echo "== Local check complete =="
