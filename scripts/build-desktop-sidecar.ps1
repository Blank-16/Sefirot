param(
    [string]$Target = ""
)
$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
if (-not $Target) {
    $Target = (& rustc -vV | Select-String '^host: ' | ForEach-Object { $_.Line.Substring(6) })
}
$Mapping = @{
    "x86_64-pc-windows-msvc" = @{ GOOS = "windows"; GOARCH = "amd64"; Ext = ".exe" }
    "aarch64-pc-windows-msvc" = @{ GOOS = "windows"; GOARCH = "arm64"; Ext = ".exe" }
    "x86_64-unknown-linux-gnu" = @{ GOOS = "linux"; GOARCH = "amd64"; Ext = "" }
    "aarch64-unknown-linux-gnu" = @{ GOOS = "linux"; GOARCH = "arm64"; Ext = "" }
    "x86_64-apple-darwin" = @{ GOOS = "darwin"; GOARCH = "amd64"; Ext = "" }
    "aarch64-apple-darwin" = @{ GOOS = "darwin"; GOARCH = "arm64"; Ext = "" }
}
if (-not $Mapping.ContainsKey($Target)) { throw "Unsupported Rust target triple: $Target" }
$Platform = $Mapping[$Target]
$OutDir = Join-Path $Root "desktop/src-tauri/binaries"
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
$Out = Join-Path $OutDir ("sefirot-server-{0}{1}" -f $Target, $Platform.Ext)
Push-Location (Join-Path $Root "backend")
try {
    $env:CGO_ENABLED = "0"
    $env:GOOS = $Platform.GOOS
    $env:GOARCH = $Platform.GOARCH
    $LdFlags = "-s -w"
    if ($Platform.GOOS -eq "windows") { $LdFlags = "-s -w -H=windowsgui" }
    go build -trimpath -ldflags $LdFlags -o $Out .
    if ($LASTEXITCODE -ne 0) { throw "Go sidecar build failed with exit code $LASTEXITCODE" }
} finally {
    Pop-Location
}
Write-Host "Built sidecar: $Out"
