#!/usr/bin/env bash
# Verifies every binary in a dist directory was built with CGO_ENABLED=0,
# and that Linux binaries are statically linked. macOS binaries always link
# libSystem dynamically; for those the cgo check is the requirement. Only
# Apple Silicon macOS and Linux are supported targets (see CLAUDE.md).
set -euo pipefail

dist="${1:-dist}"
shopt -s nullglob
bins=()
for f in "$dist"/lk-mcp-*; do
  [[ "$f" == *.mcpb ]] || bins+=("$f")
done
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
# A Claude Desktop extension carries its binary at server/lk-mcp; it is
# checked like the others, and only an Apple Silicon one is built.
for m in "$dist"/*.mcpb; do
  name="$(basename "$m")"
  if [[ "$name" != "lk-mcp-darwin-arm64.mcpb" ]]; then
    echo "FAIL $name: unsupported extension (only lk-mcp-darwin-arm64.mcpb is built)"
    fail=1
    continue
  fi
  tmp="$(mktemp -d)"
  if ! unzip -q "$m" server/lk-mcp -d "$tmp"; then
    echo "FAIL $name: no server/lk-mcp inside"
    fail=1
  elif ! go version -m "$tmp/server/lk-mcp" | grep -Eq '^[[:space:]]+build[[:space:]]+CGO_ENABLED=0$'; then
    echo "FAIL $name: server/lk-mcp lacks CGO_ENABLED=0"
    fail=1
  elif [[ "$(file -b "$tmp/server/lk-mcp")" != *"arm64"* ]]; then
    echo "FAIL $name: server/lk-mcp is not arm64"
    fail=1
  else
    echo "ok   $name: server/lk-mcp CGO_ENABLED=0 ($(file -b "$tmp/server/lk-mcp" | cut -d, -f1))"
  fi
  rm -rf "$tmp"
done
exit $fail
