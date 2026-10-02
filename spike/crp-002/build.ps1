# Builds the CRP-002 prototype and packs it as an MCPB bundle.
# Output goes to ~/crp-002-spike, outside the repository.
$ErrorActionPreference = 'Stop'
$go = (Get-Command go -ErrorAction SilentlyContinue).Source
if (-not $go) { $go = 'C:\Program Files\Go\bin\go.exe' }
$out = Join-Path $env:USERPROFILE 'crp-002-spike'
$stage = Join-Path $out 'stage'
New-Item -ItemType Directory -Force $stage | Out-Null

Push-Location $PSScriptRoot
try {
    $env:CGO_ENABLED = '0'
    $env:GOOS = 'windows'; $env:GOARCH = 'amd64'
    & $go build -trimpath -o (Join-Path $stage 'crp002-spike.exe') .
    if ($LASTEXITCODE) { throw 'windows build failed' }
    # Apple silicon. For an Intel Mac, change GOARCH to amd64 and rebuild.
    $env:GOOS = 'darwin'; $env:GOARCH = 'arm64'
    & $go build -trimpath -o (Join-Path $stage 'crp002-spike-darwin') .
    if ($LASTEXITCODE) { throw 'darwin build failed' }
} finally {
    Pop-Location
    Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
}

Copy-Item (Join-Path $stage 'crp002-spike.exe') (Join-Path $out 'crp002-spike.exe') -Force
Copy-Item (Join-Path $PSScriptRoot 'runbook.ps1') $out -Force -ErrorAction SilentlyContinue
& (Join-Path $out 'crp002-spike.exe') pack (Join-Path $PSScriptRoot 'manifest.json') (Join-Path $out 'crp002-spike.mcpb') `
    (Join-Path $stage 'crp002-spike.exe') (Join-Path $stage 'crp002-spike-darwin')
if ($LASTEXITCODE) { throw 'pack failed' }
# Mark the bundle as downloaded from the internet, as a real user's copy would be,
# so that SmartScreen and antivirus treat it the way they would treat a release.
Set-Content -Path (Join-Path $out 'crp002-spike.mcpb') -Stream Zone.Identifier -Value "[ZoneTransfer]`r`nZoneId=3`r`nHostUrl=https://github.com/Zafnok/claude-rich-presence/releases/download/spike/crp002-spike.mcpb"
Get-ChildItem $out -File | Select-Object Name, Length, LastWriteTime
