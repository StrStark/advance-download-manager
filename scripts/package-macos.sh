#!/usr/bin/env bash
# Wraps the built ADM.app into a drag-to-Applications disk image (macOS only).
# Usage: scripts/package-macos.sh <version> <out-dir>
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
version="$1"
out="$2"
app="$(ls -d "$root"/build/bin/*.app | head -1)"
[[ -d "$app" ]] || { echo "no .app in build/bin" >&2; exit 1; }
# Ad-hoc signature: required for Apple Silicon to run the binary at all.
# (A Developer ID signature + notarization would remove the first-launch warning.)
codesign --force --deep --sign - "$app"
stage="$(mktemp -d)"
cp -R "$app" "$stage/ADM.app"
ln -s /Applications "$stage/Applications"
mkdir -p "$out"
hdiutil create -volname "ADM $version" -srcfolder "$stage" -ov -format UDZO "$out/adm_${version}_macos_universal.dmg"
rm -rf "$stage"
