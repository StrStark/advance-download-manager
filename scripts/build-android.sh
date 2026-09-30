#!/usr/bin/env bash
# Builds the Android app:
#   1. the Svelte UI              (frontend/dist)
#   2. the Go engine as an .aar   (gomobile bind ./mobile)
#   3. the APKs                   (Gradle)
# Output: build/bin/adm-android-*.apk
#
# Usage: scripts/build-android.sh [debug|release]      (default: debug)
# Needs the tools from scripts/setup-android-tools.sh.
set -euo pipefail

variant="${1:-debug}"
root="$(cd "$(dirname "$0")/.." && pwd)"
[[ -f "$HOME/.adm-android-env" ]] && source "$HOME/.adm-android-env"
: "${ANDROID_HOME:?Android SDK not found; run scripts/setup-android-tools.sh first}"
command -v gomobile >/dev/null || { echo "gomobile not found; run scripts/setup-android-tools.sh" >&2; exit 1; }

echo "==> UI"
(cd "$root/frontend" && npm run build --silent)
find "$root/mobile/dist" -mindepth 1 ! -name .gitkeep -delete
cp -r "$root/frontend/dist/." "$root/mobile/dist/"

echo "==> Go engine (.aar)"
mkdir -p "$root/android/app/libs"
(cd "$root" && gomobile bind \
  -target=android/arm64,android/arm,android/amd64 \
  -androidapi 26 \
  -ldflags="-s -w" -trimpath \
  -o android/app/libs/admmobile.aar ./mobile)

echo "==> APK ($variant)"
cd "$root/android"
if [[ ! -x ./gradlew ]]; then
  # First build: create the Gradle wrapper with a throwaway Gradle download.
  ver="$(sed -n 's/.*gradle-\([0-9.]*\)-bin.zip/\1/p' gradle/wrapper/gradle-wrapper.properties)"
  tmp="$(mktemp -d)"
  curl -fL --retry 3 -o "$tmp/gradle.zip" "https://services.gradle.org/distributions/gradle-$ver-bin.zip"
  python3 -c "import zipfile,sys; zipfile.ZipFile(sys.argv[1]).extractall(sys.argv[2])" "$tmp/gradle.zip" "$tmp"
  chmod +x "$tmp/gradle-$ver/bin/gradle"
  "$tmp/gradle-$ver/bin/gradle" wrapper --gradle-version "$ver" --no-daemon
  rm -rf "$tmp"
fi
echo "sdk.dir=$ANDROID_HOME" > local.properties
task="assemble${variant^}"
./gradlew "$task" --no-daemon

mkdir -p "$root/build/bin"
for apk in app/build/outputs/apk/"$variant"/*.apk; do
  name="$(basename "$apk" | sed "s/^app-/adm-android-/")"
  cp "$apk" "$root/build/bin/$name"
  echo "   $root/build/bin/$name"
done
echo "Install on a phone with USB debugging:  adb install -r build/bin/adm-android-universal-$variant.apk"
