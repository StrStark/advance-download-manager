#!/usr/bin/env bash
# Installs the ADM desktop app for the current user (no root needed):
#   binary  -> ~/.local/bin/adm
#   icon    -> ~/.local/share/icons/hicolor/512x512/apps/adm.png
#   launcher-> ~/.local/share/applications/adm.desktop
#
# Usage: scripts/install-linux.sh [path/to/adm]   |   scripts/install-linux.sh --uninstall
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
bin_dir="$HOME/.local/bin"
app_dir="${XDG_DATA_HOME:-$HOME/.local/share}/applications"
icon_dir="${XDG_DATA_HOME:-$HOME/.local/share}/icons/hicolor/512x512/apps"

refresh() {
  command -v update-desktop-database >/dev/null && update-desktop-database -q "$app_dir" || true
  command -v gtk-update-icon-cache >/dev/null && gtk-update-icon-cache -q -t "$(dirname "$(dirname "$icon_dir")")" || true
}

if [[ "${1:-}" == "--uninstall" ]]; then
  rm -f "$bin_dir/adm" "$app_dir/adm.desktop" "$icon_dir/adm.png"
  refresh
  echo "ADM removed. Your downloads and ~/.local/share/adm (history, settings) were kept."
  exit 0
fi

src="${1:-$root/build/bin/adm}"
if [[ ! -x "$src" ]]; then
  echo "No binary at $src. Build it first: wails build -tags webkit2_41" >&2
  exit 1
fi

install -Dm755 "$src" "$bin_dir/adm"
install -Dm644 "$root/build/appicon.png" "$icon_dir/adm.png"
mkdir -p "$app_dir"
# Absolute Exec path: ~/.local/bin is not always on the launcher's PATH.
sed "s|^Exec=adm |Exec=$bin_dir/adm |" "$root/build/linux/adm.desktop" > "$app_dir/adm.desktop"
chmod 644 "$app_dir/adm.desktop"
refresh

echo "ADM installed. Find it in your app launcher, or run: $bin_dir/adm"
