#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

mkdir -p "$repo_root/build"

go build -o build/seq ./cmd/sequential/
go build -o build/coordinator ./cmd/coordinator
go build -o build/worker ./cmd/worker

for plugin_dir in "$repo_root"/plugins/*/; do
  plugin_name="$(basename "$plugin_dir")"
  go build -buildmode=plugin -o "$repo_root/build/${plugin_name}.so" "./plugins/${plugin_name}/"
done