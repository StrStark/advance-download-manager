#!/usr/bin/env bash
# Packages the browser extension for Chromium browsers and Firefox.
# Usage: scripts/package-extension.sh <version> <out-dir>
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
version="${1:-0.0.0}"
out="${2:-$root/dist}"
ext_version="${version%%-*}" # browsers want plain dotted numbers
mkdir -p "$out"
tmp="$(mktemp -d)"
for target in chrome firefox; do
  dir="$tmp/$target"
  mkdir -p "$dir"
  cp -r "$root/browser-extension/"{background.js,popup.html,popup.js,icons} "$dir/"
  src="$root/browser-extension/manifest.json"
  [[ $target == firefox ]] && src="$root/browser-extension/manifest.firefox.json"
  python3 - "$src" "$dir/manifest.json" "$ext_version" <<'PY'
import json, sys
m = json.load(open(sys.argv[1]))
m["version"] = sys.argv[3]
json.dump(m, open(sys.argv[2], "w"), indent=2)
PY
  (cd "$dir" && python3 -c "import shutil; shutil.make_archive('$out/adm-browser-extension_${version}_${target}', 'zip', '.')")
done
rm -rf "$tmp"
ls -1 "$out"/adm-browser-extension_*
