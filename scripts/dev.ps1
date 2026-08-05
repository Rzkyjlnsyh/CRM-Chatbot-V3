# Wrapper Windows (PowerShell) → launcher lintas platform
# Pemakaian:
#   powershell -ExecutionPolicy Bypass -File .\scripts\dev.ps1
#   atau:  npm run dev

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

$node = Get-Command node -ErrorAction SilentlyContinue
if (-not $node) {
  Write-Host "Node.js belum terpasang atau tidak ada di PATH." -ForegroundColor Red
  Write-Host "Install dari https://nodejs.org/ lalu buka ulang terminal."
  exit 1
}

& node (Join-Path $Root "scripts\dev.mjs") @args
exit $LASTEXITCODE
