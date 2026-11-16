$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$Temp = Join-Path ([System.IO.Path]::GetTempPath()) ("sefirot-smoke-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $Temp | Out-Null
$Binary = Join-Path $Temp "sefirot.exe"
$Ready = Join-Path $Temp "ready.txt"
$Stdout = Join-Path $Temp "stdout.log"
$Stderr = Join-Path $Temp "stderr.log"
$Process = $null
try {
    Push-Location (Join-Path $Root "backend")
    go build -o $Binary .
    Pop-Location
    $Process = Start-Process -FilePath $Binary -ArgumentList @("--data-dir", (Join-Path $Temp "data"), "--port", "0", "--ready-file", $Ready) -PassThru -RedirectStandardOutput $Stdout -RedirectStandardError $Stderr
    $Url = $null
    for ($i = 0; $i -lt 100; $i++) {
        if (Test-Path $Ready) { $ReadyLines = Get-Content $Ready
        $Url = $ReadyLines[0].Trim()
        $Token = $ReadyLines[1].Trim(); break }
        if ($Process.HasExited) { throw "SEFIROT exited early. $((Get-Content $Stderr -Raw -ErrorAction SilentlyContinue))" }
        Start-Sleep -Milliseconds 100
    }
    if (-not $Url) { throw "SEFIROT did not become ready. $((Get-Content $Stderr -Raw -ErrorAction SilentlyContinue))" }
    Invoke-WebRequest -Uri "$Url/api/health" -UseBasicParsing | Out-Null
    $Page = (Invoke-WebRequest -Uri "$Url/" -UseBasicParsing).Content
    if ($Page -notmatch "SEFIROT") { throw "SEFIROT homepage did not contain the expected title" }
    Invoke-WebRequest -Uri "$Url/api/shutdown" -Method Post -Headers @{ "X-Vault-Token" = $Token } -UseBasicParsing | Out-Null
    if (-not $Process.WaitForExit(5000)) { throw "SEFIROT did not shut down cleanly" }
    Write-Host "SEFIROT smoke test passed at $Url"
} finally {
    if ($Process -and -not $Process.HasExited) { Stop-Process -Id $Process.Id -Force }
    if (Get-Location | Where-Object { $_.Path -eq $Root }) { Pop-Location }
    Remove-Item -Recurse -Force $Temp -ErrorAction SilentlyContinue
}
