#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
name="$(jq -r '.name' "$repo_root/info.json")"
version="$(jq -r '.version' "$repo_root/info.json")"

if [[ -z "$name" || -z "$version" || "$name" == "null" || "$version" == "null" ]]; then
  echo "info.json must contain non-empty name and version" >&2
  exit 1
fi

package_name="${name}_${version}"
out_path="$repo_root/${package_name}.zip"
temp_root="$(mktemp -d)"
stage_root="$temp_root/$package_name"

mkdir -p "$stage_root"

rsync -a \
  --exclude 'server/' \
  --exclude 'deploy/' \
  --exclude '.github/' \
  --exclude 'tooling/' \
  --exclude '.git/' \
  --exclude '*.zip' \
  --exclude 'generated/index.lua' \
  "$repo_root/" "$stage_root/"

rm -f "$out_path"
(
  cd "$temp_root"
  zip -qr "$out_path" "$package_name"
)

rm -rf "$temp_root"
echo "Packaged: $out_path"
