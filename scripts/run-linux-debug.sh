#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOG_PATH="$SCRIPT_DIR/mimir-debug.log"
TEST_DIR="$HOME/mimir-test"
ARCHIVE="$SCRIPT_DIR/mimir-linux-amd64-dev.tar.gz"

{
  echo "== Mimir Linux debug =="
  date -Is
  echo "user: $(id)"
  echo "pwd: $(pwd)"
  echo "archive: $ARCHIVE"

  mkdir -p "$TEST_DIR"
  rm -f "$TEST_DIR/mimir"
  tar -C "$TEST_DIR" -xzf "$ARCHIVE"
  chmod +x "$TEST_DIR/mimir"

  echo "== ldd missing =="
  ldd "$TEST_DIR/mimir" | grep "not found" || true

  echo "== app output =="
  cd "$TEST_DIR"
  ./mimir
} >"$LOG_PATH" 2>&1
