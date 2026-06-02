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
ARTIFACT_NAME="mimir-darwin-universal-$VERSION.zip"
ARTIFACT_PATH="$RELEASE_DIR/$ARTIFACT_NAME"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "macOS release builds must run on macOS." >&2
  exit 1
fi

if [[ "${GOTOOLCHAIN:-}" == "" || "${GOTOOLCHAIN:-}" == "local" ]]; then
  export GOTOOLCHAIN="auto"
fi
export GOCACHE="${GOCACHE:-/tmp/mimir-go-build-cache}"
export GOMODCACHE="${GOMODCACHE:-/tmp/mimir-go-mod-cache}"

cd "$PROJECT_ROOT"
mkdir -p "$RELEASE_DIR"

mapfile -t GO_PACKAGES < <(find . -path './frontend' -prune -o -path './build' -prune -o -name '*.go' -printf '%h\n' | sort -u)

# Build the frontend first so go:embed (all:frontend/dist) and the Wails build
# both find frontend/dist. go test compiles the main package, which embeds dist.
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

echo "== Go tests =="
go test "${GO_PACKAGES[@]}"

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
# -s: skip Wails' own frontend build; frontend/dist is already built above.
wails build -s -ldflags "$LDFLAGS"

APP_PATH="$PROJECT_ROOT/build/bin/mimir.app"
BINARY_PATH="$PROJECT_ROOT/build/bin/mimir"
rm -f "$ARTIFACT_PATH"

if [[ -d "$APP_PATH" ]]; then
  echo "== Packaging $ARTIFACT_NAME =="
  ditto -c -k --keepParent "$APP_PATH" "$ARTIFACT_PATH"
elif [[ -x "$BINARY_PATH" ]]; then
  echo "== Packaging $ARTIFACT_NAME =="
  ditto -c -k "$BINARY_PATH" "$ARTIFACT_PATH"
else
  echo "Expected macOS app or binary not found in build/bin." >&2
  exit 1
fi

echo "== Checksums =="
(
  cd "$RELEASE_DIR"
  if [[ -f checksums.txt ]]; then
    grep -v "  $ARTIFACT_NAME$" checksums.txt > checksums.txt.tmp || true
    mv checksums.txt.tmp checksums.txt
  fi
  shasum -a 256 "$ARTIFACT_NAME" >> checksums.txt
)

echo "Release artifact:"
echo "$ARTIFACT_PATH"
