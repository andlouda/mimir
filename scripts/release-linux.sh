#!/usr/bin/env bash
set -euo pipefail

VERSION="dev"
SKIP_AUDIT="false"
UPDATE_REPOSITORY="${UPDATE_REPOSITORY:-}"

for arg in "$@"; do
  case "$arg" in
    --skip-audit)
      SKIP_AUDIT="true"
      ;;
    --update-repo=*)
      UPDATE_REPOSITORY="${arg#*=}"
      ;;
    *)
      VERSION="$arg"
      ;;
  esac
done
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RELEASE_DIR="$PROJECT_ROOT/build/release"
ARTIFACT_NAME="mimir-linux-amd64-$VERSION.tar.gz"
ARTIFACT_PATH="$RELEASE_DIR/$ARTIFACT_NAME"

if [[ "${GOTOOLCHAIN:-}" == "" || "${GOTOOLCHAIN:-}" == "local" ]]; then
  export GOTOOLCHAIN="auto"
fi
export GOCACHE="${GOCACHE:-/tmp/mimir-go-build-cache}"
export GOMODCACHE="${GOMODCACHE:-/tmp/mimir-go-mod-cache}"

cd "$PROJECT_ROOT"
mkdir -p "$RELEASE_DIR"

mapfile -t GO_PACKAGES < <(find . -path './frontend' -prune -o -path './build' -prune -o -name '*.go' -printf '%h\n' | sort -u)

echo "== Go tests =="
go test "${GO_PACKAGES[@]}"

echo "== Go race tests =="
go test -race "${GO_PACKAGES[@]}"

echo "== Frontend install =="
(
  cd frontend
  npm ci
  echo "== Frontend build =="
  npm run build
  if [[ "$SKIP_AUDIT" == "true" ]]; then
    echo "Skipping npm audit."
  else
    echo "== npm audit =="
    npm audit --audit-level=moderate
  fi
)

if command -v govulncheck >/dev/null 2>&1; then
  echo "== govulncheck =="
  govulncheck "${GO_PACKAGES[@]}"
else
  echo "Skipping govulncheck; install with: go install golang.org/x/vuln/cmd/govulncheck@latest"
fi

echo "== Wails build =="
LDFLAGS="-X main.AppVersion=$VERSION"
if [[ "$UPDATE_REPOSITORY" != "" ]]; then
  LDFLAGS="$LDFLAGS -X main.UpdateRepository=$UPDATE_REPOSITORY"
fi
wails build -ldflags "$LDFLAGS"

BINARY_PATH="$PROJECT_ROOT/build/bin/mimir"
if [[ ! -x "$BINARY_PATH" ]]; then
  echo "Expected Linux binary not found or not executable: $BINARY_PATH" >&2
  exit 1
fi

echo "== Packaging $ARTIFACT_NAME =="
tar -C "$PROJECT_ROOT/build/bin" -czf "$ARTIFACT_PATH" mimir

echo "== Checksums =="
(
  cd "$RELEASE_DIR"
  if [[ -f checksums.txt ]]; then
    grep -v "  $ARTIFACT_NAME$" checksums.txt > checksums.txt.tmp || true
    mv checksums.txt.tmp checksums.txt
  fi
  sha256sum "$ARTIFACT_NAME" >> checksums.txt
)

echo "Release artifact:"
echo "$ARTIFACT_PATH"
