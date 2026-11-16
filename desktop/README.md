# SEFIROT desktop shell

The desktop application is a Tauri v2 shell around the same Go server used by browser mode. It starts the Go sidecar, waits for its loopback URL, navigates the native webview to that URL, and terminates the sidecar when the desktop application exits. The vault and SQLite schema remain owned by the Go core. By default the shell uses Tauri’s app-local data directory; set `SEFIROT_DATA_DIR` before launch to reuse an existing legacy project directory or choose another data root.

## Prerequisites

- Go 1.25+
- Rust stable and the platform's Tauri prerequisites
- Node.js 20.19+ or 22.12+

## Build

From the repository root, run the platform build script:

```sh
# macOS / Linux / WSL
./scripts/build-desktop.sh

# Windows PowerShell
.\scripts\build-desktop.ps1
```

For a cross-target build, pass the Rust target triple to the script, for example `./scripts/build-desktop.sh aarch64-apple-darwin`. Install that Rust target first. The script builds local frontend assets, the matching Go sidecar, and the Tauri installer.

For a cross-target build, pass the Rust target triple to the script and install that Rust target before invoking the Tauri build. Supported mappings are listed in the script. Windows uses WebView2; Linux requires WebKitGTK development packages; macOS uses WKWebView.

The Tauri wrapper is a first implementation scaffold. Its Rust build and native runtime still need validation on Windows, Linux, and macOS before distributing installers.

## Platform notes

- **Windows:** install the WebView2 Runtime and Visual Studio C++ Build Tools. The integrated terminal uses CrossPTY-backed Windows ConPTY and requires Windows 10 1809 or newer. PowerShell 7 is preferred, with Windows PowerShell and `cmd.exe` fallbacks.
- **WSL2:** run the Linux server/desktop build inside the WSL distribution. With WSLg, the Linux desktop window can be displayed on the Windows desktop. For complete terminal/API integration, run the Linux SEFIROT build inside WSL; its local server and PTY then share the same Linux networking and filesystem context. The Windows app also exposes a WSL terminal profile, but in WSL2 NAT networking that shell may not be able to reach the Windows-hosted loopback API; mirrored networking or a native WSL instance is needed. Keep the active vault and SQLite database in the WSL Linux filesystem for predictable file watching and SQLite behavior; use `/mnt/c` only for explicitly selected external folders.
- **Linux:** install the Tauri WebKitGTK 4.1 and build dependencies for your distribution (on Ubuntu: `libwebkit2gtk-4.1-dev`, `build-essential`, `curl`, `wget`, `file`, `libxdo-dev`, `libssl-dev`, and `librsvg2-dev`).
- **macOS:** install Xcode Command Line Tools. The build script supports Intel (`x86_64-apple-darwin`) and Apple Silicon (`aarch64-apple-darwin`).

Do not run two SEFIROT instances against the same live vault simultaneously. The HTML vault is portable, but each installation should keep its own `index.db` in its local application data directory. Do not place the SQLite database on a network share or use one live database from both Windows and WSL.
