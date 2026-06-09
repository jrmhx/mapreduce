#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

mkdir -p "$repo_root/build"

for plugin_dir in "$repo_root"/plugins/*/; do
  plugin_name="$(basename "$plugin_dir")"
  go build -buildmode=plugin -o "$repo_root/build/${plugin_name}.so" "./plugins/${plugin_name}/"
done