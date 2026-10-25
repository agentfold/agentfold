#!/usr/bin/env bash
# Fails when generated code is out of date with its sources
# (SQL migrations/queries for sqlc, api/openapi.yaml for oapi-codegen).
set -euo pipefail

cd "$(dirname "$0")/.."

sqlc diff

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
sed "s|^output: .*|output: $tmp/openapi.gen.go|" oapi-codegen.yaml >"$tmp/config.yaml"
oapi-codegen -config "$tmp/config.yaml" ../api/openapi.yaml
if ! diff -q "$tmp/openapi.gen.go" internal/transport/rest/openapi/openapi.gen.go >/dev/null; then
  echo "internal/transport/rest/openapi is out of date: run 'mise run generate'" >&2
  exit 1
fi
