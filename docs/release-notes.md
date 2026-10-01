## Downloads

| Platform | File |
| --- | --- |
| 🪟 **Windows** 10/11 | `adm_*_windows_amd64_setup.exe` |
| 🐧 **Debian / Ubuntu / Mint / Pop!_OS** | `adm_*_linux_amd64.deb` (install with `sudo apt install ./adm_*_linux_amd64.deb`) |
| 🐧 Other Linux (portable) | `adm_*_linux_amd64`: `chmod +x` and run |
| 🍎 **macOS** (Apple Silicon + Intel) | `adm_*_macos_universal.dmg` |
| 🤖 **Android** 8.0+ | `adm_*_android_arm64-v8a.apk` (most phones). Older phones: `armeabi-v7a`. Not sure? `universal`. |
| 🐳 **Server / Docker** | `docker pull ghcr.io/strstark/advance-download-manager:latest`, or `adm-server_*_linux_<arch>` |
| 🧩 **Browser extension** | `adm-browser-extension_*_chrome.zip` (Chrome, Edge, Brave, Vivaldi) · `adm-browser-extension_*_firefox.zip` |

Verify downloads with `SHA256SUMS`.

## First launch

- **Windows:** SmartScreen may say "Windows protected your PC" because the installer isn't code-signed yet. Click **More info → Run anyway**.
- **macOS:** the app isn't notarized yet. Open the DMG, drag ADM to Applications, then **right-click ADM → Open** (or *System Settings → Privacy & Security → Open Anyway*).
- **Android:** allow your browser or file manager to *install unknown apps* when asked.

From now on ADM checks for new versions itself and updates from **Settings → About & updates**.

## Browser extension

Unzip `adm-browser-extension_*_chrome.zip`, open `chrome://extensions`, enable **Developer mode**, click **Load unpacked**, and pick the folder. ADM registers itself with the browser automatically; the extension's popup shows **Connected**.
