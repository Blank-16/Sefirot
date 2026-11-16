param([string]$Target = "")
$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Push-Location $Root
try {
    npm install
    if ($LASTEXITCODE -ne 0) { throw "Root npm install failed" }
    npm run frontend:build
    if ($LASTEXITCODE -ne 0) { throw "Frontend asset build failed" }
    if ($Target) {
        & (Join-Path $PSScriptRoot "build-desktop-sidecar.ps1") -Target $Target
    } else {
        & (Join-Path $PSScriptRoot "build-desktop-sidecar.ps1")
    }
    if ($LASTEXITCODE -ne 0) { throw "Go sidecar build failed" }
    npm --prefix desktop install
    if ($LASTEXITCODE -ne 0) { throw "Desktop npm install failed" }
    if ($Target) {
        npm --prefix desktop run build -- --target $Target
    } else {
        npm --prefix desktop run build
    }
    if ($LASTEXITCODE -ne 0) { throw "Tauri desktop build failed" }
} finally {
    Pop-Location
}
