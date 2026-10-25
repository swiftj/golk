#!/usr/bin/env bash
# Verifies every binary in a dist directory was built with CGO_ENABLED=0,
# and that Linux binaries are statically linked. macOS binaries always link
# libSystem dynamically; for those the cgo check is the requirement. Only
# Apple Silicon macOS and Linux are supported targets (see CLAUDE.md).
set -euo pipefail

dist="${1:-dist}"
shopt -s nullglob
bins=("$dist"/lk-mcp-*)
if [ ${#bins[@]} -eq 0 ]; then
  echo "verify-static: no binaries in $dist (run make build)" >&2
  exit 1
fi

fail=0
for b in "${bins[@]}"; do
  name="$(basename "$b")"
  if ! go version -m "$b" | grep -Eq '^[[:space:]]+build[[:space:]]+CGO_ENABLED=0$'; then
    echo "FAIL $name: CGO_ENABLED=0 not recorded in build info"
    fail=1
    continue
  fi
  case "$name" in
    lk-mcp-linux-*)
      desc="$(file -b "$b")"
      if [[ "$desc" != *"statically linked"* ]]; then
        echo "FAIL $name: not statically linked: $desc"
        fail=1
        continue
      fi
      echo "ok   $name: CGO_ENABLED=0, statically linked"
      ;;
    lk-mcp-darwin-arm64*)
      echo "ok   $name: CGO_ENABLED=0 ($(file -b "$b" | cut -d, -f1))"
      ;;
    *)
      echo "FAIL $name: unsupported target (only darwin/arm64 and linux are built)"
      fail=1
      ;;
  esac
done
exit $fail
