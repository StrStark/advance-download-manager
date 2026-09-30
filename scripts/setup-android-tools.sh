#!/usr/bin/env bash
# One-time setup of everything needed to build the Android app, installed
# under your home folder (no root):
#   JDK 17            -> ~/.local/jdk-17
#   Android SDK + NDK -> ~/Android/Sdk
#   gomobile          -> ~/go/bin
# Download size is roughly 2 GB (the NDK is most of it).
set -euo pipefail

JDK_URL="https://api.adoptium.net/v3/binary/latest/17/ga/linux/x64/jdk/hotspot/normal/eclipse"
CMDLINE_TOOLS_URL="https://dl.google.com/android/repository/commandlinetools-linux-13114758_latest.zip"
ANDROID_HOME="${ANDROID_HOME:-$HOME/Android/Sdk}"
JAVA_HOME_DIR="$HOME/.local/jdk-17"
NDK_VERSION="27.2.12479018"
PLATFORM="android-35"
BUILD_TOOLS="35.0.0"

say() { printf '\n\033[1;34m==> %s\033[0m\n' "$*"; }

if [[ ! -x "$JAVA_HOME_DIR/bin/java" ]]; then
  say "Installing JDK 17 to $JAVA_HOME_DIR"
  mkdir -p "$JAVA_HOME_DIR"
  curl -fL --retry 3 "$JDK_URL" | tar -xz -C "$JAVA_HOME_DIR" --strip-components=1
fi
export JAVA_HOME="$JAVA_HOME_DIR" PATH="$JAVA_HOME_DIR/bin:$PATH"

if [[ ! -x "$ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager" ]]; then
  say "Installing Android command-line tools to $ANDROID_HOME"
  tmp="$(mktemp -d)"
  curl -fL --retry 3 -o "$tmp/tools.zip" "$CMDLINE_TOOLS_URL"
  python3 -c "import zipfile,sys; zipfile.ZipFile(sys.argv[1]).extractall(sys.argv[2])" "$tmp/tools.zip" "$tmp"
  mkdir -p "$ANDROID_HOME/cmdline-tools"
  rm -rf "$ANDROID_HOME/cmdline-tools/latest"
  mv "$tmp/cmdline-tools" "$ANDROID_HOME/cmdline-tools/latest"
  chmod +x "$ANDROID_HOME/cmdline-tools/latest/bin/"*
  rm -rf "$tmp"
fi
sdkmanager="$ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager"

say "Accepting Android SDK licenses"
yes | "$sdkmanager" --sdk_root="$ANDROID_HOME" --licenses >/dev/null || true

say "Installing platform $PLATFORM, build-tools $BUILD_TOOLS, NDK $NDK_VERSION (large)"
"$sdkmanager" --sdk_root="$ANDROID_HOME" \
  "platform-tools" "platforms;$PLATFORM" "build-tools;$BUILD_TOOLS" "ndk;$NDK_VERSION"

say "Installing gomobile"
export PATH="$HOME/.local/go/bin:$HOME/go/bin:$PATH"
go install golang.org/x/mobile/cmd/gomobile@latest
go install golang.org/x/mobile/cmd/gobind@latest
cd "$(dirname "$0")/.."
go get golang.org/x/mobile/bind@latest
ANDROID_NDK_HOME="$ANDROID_HOME/ndk/$NDK_VERSION" gomobile init

cat > "$HOME/.adm-android-env" <<ENV
export JAVA_HOME="$JAVA_HOME_DIR"
export ANDROID_HOME="$ANDROID_HOME"
export ANDROID_NDK_HOME="$ANDROID_HOME/ndk/$NDK_VERSION"
export PATH="\$JAVA_HOME/bin:\$ANDROID_HOME/platform-tools:\$HOME/.local/go/bin:\$HOME/go/bin:\$PATH"
ENV

say "Done. Environment saved to ~/.adm-android-env; next: scripts/build-android.sh"
