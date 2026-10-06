# Shows a presence with a repository button in a real Discord, for the manual
# check of CRP-048. See crp-048-repository-link.md beside this file.
#
# It runs the binary as Claude Code would, with a home directory, a
# configuration file and a runtime directory of its own under %TEMP%, so it
# does not read or change your own configuration and does not meet a presence
# host that is already running. It holds the presence for -Seconds, then ends.
#
#   pwsh -File docs\research\crp-048-discord-check.ps1 -ApplicationId <id>
param(
    [Parameter(Mandatory = $true)][string]$ApplicationId,
    [string]$Binary = (Join-Path $PSScriptRoot '..\..\bin\rich-presence.exe'),
    [string]$Link = 'https://github.com/Zafnok/claude-rich-presence',
    [ValidateSet('minimal', 'standard', 'full')][string]$Privacy = 'standard',
    [int]$Seconds = 300,
    # For trying the script itself: an endpoint name that is not Discord's.
    [string]$Endpoint = ''
)
$ErrorActionPreference = 'Stop'

$root = Join-Path ([IO.Path]::GetTempPath()) 'rp-link-check'
$project = Join-Path $root 'project'
$configDir = Join-Path $root '.rich-presence'
New-Item -ItemType Directory -Force $project, $configDir, (Join-Path $root 'run') | Out-Null

@{
    discord_application_id = $ApplicationId
    privacy                = 'minimal'
    log_level              = 'debug'
    projects               = @(@{ path = $project; privacy = $Privacy; link = $Link; name = 'Link check' })
} | ConvertTo-Json -Depth 4 | Set-Content -Encoding utf8 (Join-Path $configDir 'config.json')

$env:USERPROFILE = $root
$env:HOME = $root
$env:RICH_PRESENCE_RUNTIME_DIR = Join-Path $root 'run'
if ($Endpoint) { $env:RICH_PRESENCE_DISCORD_ENDPOINT = $Endpoint }

function Message($id, $method, $params) {
    $m = [ordered]@{ jsonrpc = '2.0' }
    if ($id) { $m.id = $id }
    $m.method = $method
    if ($params) { $m.params = $params }
    $m | ConvertTo-Json -Depth 6 -Compress
}
function Hook($id, $event) {
    Message $id 'tools/call' @{ name = 'presence_event'; arguments = @{ event = $event; session_id = 'link-check'; cwd = $project } }
}

& {
    Message 1 'initialize' @{ protocolVersion = '2025-06-18'; capabilities = @{}; clientInfo = @{ name = 'claude-code'; version = '0' } }
    Message $null 'notifications/initialized' $null
    Hook 2 'UserPromptSubmit'
    Start-Sleep -Seconds 5
    Message 3 'tools/call' @{ name = 'presence_status'; arguments = @{} }
    Start-Sleep -Seconds $Seconds
} | & $Binary mcp

Write-Host "Done. The log is under $configDir\logs."
