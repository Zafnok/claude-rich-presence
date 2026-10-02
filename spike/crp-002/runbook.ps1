# CRP-002 runbook for Windows. Walks the owner through the steps that need the
# Claude Desktop user interface and records each one in the prototype's log.
#
# Run it from a PowerShell window opened from the Start menu. Do NOT run it from
# a terminal inside Claude Desktop or Claude Code: the point of several steps is
# to compare a process outside Claude Desktop with one inside it.
#
#   powershell -ExecutionPolicy Bypass -File "$env:USERPROFILE\crp-002-spike\runbook.ps1"
#
# The first run, on 2026-10-02, left out everything that needs Claude Desktop to
# be quit. Those steps are a second, shorter list:
#
#   powershell -ExecutionPolicy Bypass -File "$env:USERPROFILE\crp-002-spike\runbook.ps1" -Later
#
# To resume after an interruption, add:  -StartAt 3
param([int]$StartAt = 0, [switch]$Later)

$dir = Join-Path $env:USERPROFILE 'crp-002-spike'
$exe = Join-Path $dir 'crp002-spike.exe'
$log = Join-Path $dir 'spike.log'
$trig = Join-Path $dir 'triggers'
$bundle = Join-Path $dir 'crp002-spike.mcpb'
$leaf = 'rich-presence-crp002'

function Mark([string]$text) { & $exe mark $text }

function Say([string]$text) { Write-Host $text -ForegroundColor Cyan }

function Step([string]$id, [string]$text) {
    Write-Host ''
    Say "[$id] $text"
    Mark "$id PROMPT: $text"
    Read-Host '      Press Enter when you have done it' | Out-Null
    Mark "$id DONE"
}

function Ask([string]$id, [string]$question) {
    $answer = Read-Host "      $question"
    Mark "$id ANSWER: $question => $answer"
    return $answer
}

function Pause-For([int]$seconds, [string]$why) {
    Write-Host "      waiting $seconds s ($why)" -ForegroundColor DarkGray
    Start-Sleep -Seconds $seconds
}

function When($t) {
    if ($t -is [datetime]) { return $t.ToUniversalTime() }
    return [datetime]::Parse($t, [cultureinfo]::InvariantCulture, [System.Globalization.DateTimeStyles]::AdjustToUniversal)
}

# Prints the servers Claude has started in this run and whether each is still alive.
function Servers([string]$id) {
    $events = @(Get-Content $log -ErrorAction SilentlyContinue | ForEach-Object { try { $_ | ConvertFrom-Json } catch { } })
    $from = 0
    for ($i = 0; $i -lt $events.Count; $i++) {
        if ($events[$i].ev -eq 'mark' -and $events[$i].text -match '^(RUNBOOK|LATER) START') { $from = $i }
    }
    $events = @($events | Select-Object -Skip $from)
    $now = [datetime]::UtcNow
    $rows = foreach ($s in ($events | Where-Object { $_.role -eq 'mcp' -and $_.ev -eq 'start' })) {
        $mine = @($events | Where-Object { $_.pid -eq $s.pid -and $_.role -eq 'mcp' })
        $exited = $mine | Where-Object { $_.ev -eq 'exit' } | Select-Object -First 1
        $init = $mine | Where-Object { $_.ev -eq 'mcp_initialize' } | Select-Object -First 1
        $age = [int]($now - (When $mine[-1].t)).TotalSeconds
        $state = 'alive'
        if ($exited) { $state = "exited ($($exited.reason))" } elseif ($age -gt 12) { $state = "gone, no heartbeat for $age s" }
        $parent = ''
        if ($s.sys.ancestry.Count -gt 1) { $parent = Split-Path -Leaf (($s.sys.ancestry[1] -split ' ', 2)[1]) }
        [pscustomobject]@{ pid = $s.pid; started = (When $s.t).ToString('HH:mm:ss'); state = $state; client = $init.clientInfo.name; parent = $parent }
    }
    if ($rows) { $rows | Format-Table -AutoSize | Out-String | Write-Host } else { Write-Host '      no server has started yet' }
    $alive = @($rows | Where-Object { $_.state -eq 'alive' } | ForEach-Object { $_.pid }) -join ','
    Mark "$id SNAPSHOT alive=[$alive] total=$(@($rows).Count)"
}

# Removes the test directories. Never while a copy of the prototype is running:
# deleting the socket file of a live listener leaves a host nobody can reach,
# which spoiled one step of the first run.
function Clean-Candidates {
    $running = @(Get-Process -Name crp002-spike -ErrorAction SilentlyContinue)
    if ($running.Count) {
        Mark "CLEAN skipped: $($running.Count) prototype process(es) still running"
        Write-Host "      clean-up skipped: $($running.Count) prototype process(es) still running" -ForegroundColor Yellow
        return
    }
    $paths = @(
        (Join-Path $env:LOCALAPPDATA $leaf),
        (Join-Path $env:TEMP $leaf),
        (Join-Path $env:USERPROFILE ".$leaf")
    )
    $paths += @(Get-ChildItem (Join-Path $env:LOCALAPPDATA 'Packages') -Directory -Filter 'Claude_*' -ErrorAction SilentlyContinue |
        ForEach-Object { Join-Path $_.FullName "LocalCache\Local\$leaf" })
    foreach ($p in $paths) {
        if (Test-Path $p) { Remove-Item $p -Recurse -Force -ErrorAction SilentlyContinue }
    }
    Mark "CLEAN candidates removed; still present: [$(@($paths | Where-Object { Test-Path $_ }) -join ', ')]"
}

function Trigger([string]$name) { Set-Content -Path (Join-Path $trig $name) -Value (Get-Date -Format o) }

function Desktop-Processes { @(Get-Process -Name claude -ErrorAction SilentlyContinue | Where-Object { $_.Path -like '*WindowsApps*' }).Count }

function Preflight([string]$label, [string[]]$intro) {
    $id = Get-CimInstance Win32_Process -Filter "ProcessId=$PID"
    $inside = $false
    for ($i = 0; $i -lt 12 -and $id; $i++) {
        if ($id.Name -match '^claude') { $inside = $true }
        $id = Get-CimInstance Win32_Process -Filter "ProcessId=$($id.ParentProcessId)"
    }
    if ($inside) { Write-Host 'This window was started by Claude. Open PowerShell from the Start menu and run the script there.' -ForegroundColor Red; exit 1 }
    if (-not (Test-Path $exe) -or -not (Test-Path $bundle)) { Write-Host "Missing $exe or $bundle. Run build.ps1 first." -ForegroundColor Red; exit 1 }

    foreach ($line in $intro) { Say $line }
    Read-Host 'Press Enter to begin, or Ctrl+C to stop' | Out-Null

    $claude = Get-AppxPackage -Name Claude -ErrorAction SilentlyContinue
    $os = Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion'
    $defender = try { (Get-MpComputerStatus -ErrorAction Stop).RealTimeProtectionEnabled } catch { 'unknown' }
    $zone = try { (Get-Content $bundle -Stream Zone.Identifier -ErrorAction Stop) -join ' ' } catch { 'none' }
    Mark "$label START claudeDesktop=$($claude.Version) packageFamily=$($claude.PackageFamilyName) windows=$($os.DisplayVersion) build=$($os.CurrentBuild).$($os.UBR) powershell=$($PSVersionTable.PSVersion) defenderRealtime=$defender bundleZone=[$zone] discordProcesses=$(@(Get-Process -Name Discord* -ErrorAction SilentlyContinue).Count)"
    Trigger 'stop-peer'; Start-Sleep -Seconds 2
    Get-ChildItem $trig -File -ErrorAction SilentlyContinue | Remove-Item -Force
    Clean-Candidates
}

function Install-Prompt([string]$id) {
    Say ''
    Say "[$id] Install the bundle. While you do, watch for three things:"
    Say '       a) any Windows SmartScreen or antivirus pop-up'
    Say '       b) what Claude''s install dialog says about trust, signing or verification'
    Say '       c) any black console window appearing or flashing'
    Say "     Install: double-click  $bundle"
    Say '       (or Claude Desktop > Settings > Extensions > Advanced settings > Install Extension...)'
    Say '     In the settings form: keep the defaults. Type any word into "Sample secret".'
    Say '     If you have a Discord application id, paste it into "Discord application id". Otherwise leave it empty.'
    Say '     Finish the install and make sure the extension is switched on. Do not open a chat yet.'
    Mark "$id PROMPT: install the bundle"
    Read-Host '      Press Enter when the extension is installed and enabled' | Out-Null
    Mark "$id DONE"
}

# Watches the servers after the app is told to quit, for up to 110 seconds.
function Watch-Quit {
    Say '      Watching the server for up to 110 seconds. It is set to linger for 90 seconds after its input closes.'
    for ($i = 0; $i -lt 22; $i++) {
        Start-Sleep -Seconds 5
        $tail = @(Get-Content $log -Tail 40 | ForEach-Object { try { $_ | ConvertFrom-Json } catch { } } | Where-Object { $_.role -eq 'mcp' })
        if ($tail | Where-Object { $_.ev -eq 'exit' -and $_.reason -like 'linger*' }) { break }
        if ($tail.Count -and ([datetime]::UtcNow - (When $tail[-1].t)).TotalSeconds -gt 20) { break }
    }
}

function Terminal-First([string]$id) {
    Say ''
    Say "[$id] Automatic: clearing the test directories and starting a copy from this terminal FIRST, so it holds the lock."
    Clean-Candidates
    $script:peer = Start-Process -FilePath $exe -ArgumentList 'peer' -WindowStyle Hidden -PassThru
    Mark "$id terminal peer started first pid=$($script:peer.Id)"
    Start-Sleep -Seconds 3
    Step "${id}a" 'Start Claude Desktop from the Start menu. Do NOT click on or open any chat. Wait until the window has fully loaded.'
    Ask "${id}a" 'Did a console window appear or flash during launch? (y/n)' | Out-Null
    Pause-For 20 'has the server started without a chat?'; Servers "${id}a"
    Trigger 'discord'; Pause-For 3 'Discord probe'
    $kill = Ask "${id}b" 'Optional: force-kill Claude Desktop now, to see whether its server dies with it? This is like a crash. (y/n)'
    if ($kill -match '^y') {
        Mark "${id}b force kill"
        Get-Process -Name claude -ErrorAction SilentlyContinue | Where-Object { $_.Path -like '*WindowsApps*' } | Stop-Process -Force -ErrorAction SilentlyContinue
        Pause-For 20 'watching the server'; Servers "${id}b"
        Mark "${id}b desktopProcessesLeft=$(Desktop-Processes)"
    }
}

function Finish([string]$id, [string]$label) {
    Trigger 'stop-peer'; Start-Sleep -Seconds 2
    Get-Process -Name crp002-spike -ErrorAction SilentlyContinue | Where-Object { $_.Id -eq $script:peer.Id } | Stop-Process -Force -ErrorAction SilentlyContinue
    Step $id 'Last one. Start Claude Desktop if it is not running, then uninstall the extension: Settings > Extensions > "CRP-002 spike (throwaway)" > Uninstall. If you prefer to keep it for now, just press Enter.'
    Ask $id 'Did you uninstall it? (y/n)' | Out-Null
    Pause-For 10 'watching the server'; Servers $id
    Mark "$label END"
    Say ''
    Say "Done. The log is $log"
    Say 'Go back to the Claude Code session for CRP-002 (resume it if it was interrupted) and say:  runbook done'
}

# ---------------------------------------------------------------------------
# The first run.

$steps = @(
    { # 0
        Preflight 'RUNBOOK' @(
            'CRP-002 runbook. About 20 minutes. It asks you to do things in Claude Desktop and press Enter after each.',
            'NOTE: parts 8 and 9 quit Claude Desktop. Every Claude Code session running inside the app is interrupted',
            'and can be resumed afterwards. Pick a moment when that is fine.')
    },
    { # 1
        Step 'S1' 'If the Discord desktop app is installed, start it now and leave it running. Then make sure Claude Desktop is running.'
        Ask 'S1' 'Is the Discord desktop app running? (y/n)' | Out-Null
    },
    { # 2
        Install-Prompt 'S2'
        Ask 'S2' 'Any SmartScreen or antivirus prompt? Describe it, or type none' | Out-Null
        Ask 'S2' 'What did Claude''s install dialog say about trust or signing? A few words, or none' | Out-Null
        Ask 'S2' 'Did a console window appear or flash? (y/n)' | Out-Null
        Pause-For 12 'letting the server start'
        Servers 'S2'
    },
    { # 3
        Say ''
        Say '[S3] Automatic: running a second copy from this terminal to see whether it shares the lock and socket.'
        Mark 'S3 terminal peer, Desktop first'
        & $exe peer --once
        Trigger 'discord'
        Pause-For 3 'Discord probe'
    },
    { # 4
        Step 'S4a' 'In Claude Desktop, on the CHAT tab, open a NEW chat. Do not send anything yet.'
        Pause-For 6 'settling'; Servers 'S4a'
        Step 'S4b' 'In that chat, send:  hi'
        Pause-For 6 'settling'; Servers 'S4b'
        Step 'S4c' 'In the same chat, send:  Please call the spike_status tool and tell me what it returns.   Approve the tool if Claude asks.'
        Ask 'S4c' 'Did Claude call the tool and show a result? Was there a permission prompt? (few words)' | Out-Null
        Servers 'S4c'
        Step 'S4d' 'Open a SECOND new chat and send:  hi'
        Pause-For 6 'settling'; Servers 'S4d'
    },
    { # 5
        Step 'S5' 'Go to the Code tab. Start a NEW local session in any folder and send:  List any MCP tools whose name contains "spike". Do not call them.'
        Ask 'S5' 'Did the Code session report a spike tool? (y/n/unsure)' | Out-Null
        Servers 'S5'
        Step 'S5b' 'Optional. If you use Cowork: start a Cowork task and send:  Call the spike_status tool.   If you do not use Cowork, just press Enter.'
        Ask 'S5b' 'Did you run the Cowork step, and did the tool work there? (few words, or skipped)' | Out-Null
        Servers 'S5b'
    },
    { # 6
        Step 'S6a' 'Minimise the Claude Desktop window.'
        Pause-For 15 'watching the server'; Servers 'S6a'
        Step 'S6b' 'Restore the window, then CLOSE it with the X button. Do not quit from the tray.'
        Pause-For 20 'watching the server'; Servers 'S6b'
        Ask 'S6b' 'After clicking X, is Claude still in the system tray? (y/n)' | Out-Null
        Step 'S6c' 'Open the Claude Desktop window again (tray icon or Start menu).'
        Pause-For 10 'settling'; Servers 'S6c'
    },
    { # 7
        Say ''
        Say '[S7a] Automatic: telling the server to exit by itself with code 0.'
        Mark 'S7a trigger exit-0'; Trigger 'exit-0'
        Pause-For 25 'watching for a restart'; Servers 'S7a'
        Ask 'S7a' 'Did Claude Desktop show an error or notification about the extension? Text, or none' | Out-Null
        Step 'S7b' 'In a chat, send:  Please call the spike_status tool again.'
        Ask 'S7b' 'Did the tool call work? (y/n, plus any error text)' | Out-Null
        Servers 'S7b'
        Step 'S7c' 'If the list above shows no server alive: switch the extension off and on again in Settings > Extensions. If one is alive, just press Enter.'
        Pause-For 10 'settling'; Servers 'S7c'
        Say ''
        Say '[S7d] Automatic: telling the server to exit by itself with code 1, like a crash.'
        Mark 'S7d trigger exit-1'; Trigger 'exit-1'
        Pause-For 25 'watching for a restart'; Servers 'S7d'
        Ask 'S7d' 'Did Claude Desktop show an error or notification this time? Text, or none' | Out-Null
        Step 'S7e' 'If the list above shows no server alive: switch the extension off and on again. If one is alive, just press Enter.'
        Pause-For 10 'settling'; Servers 'S7e'
    },
    { # 8
        Step 'S8' 'Settings > Extensions > "CRP-002 spike (throwaway)" > Configure. Turn OFF "Expose the diagnostic tool". Turn ON "Linger after the app closes the connection". Change "Sample number" to 7. Save.'
        Pause-For 15 'watching for a restart'; Servers 'S8'
        Ask 'S8' 'Any warning now that the extension has no tools? Text, or none' | Out-Null
        Say ''
        Say '      The next step quits Claude Desktop. Claude Code sessions inside it are interrupted; resume them afterwards.'
        Step 'S8q' 'Quit Claude Desktop completely: right-click its tray icon and choose Quit (or File > Exit).'
        Watch-Quit
        Servers 'S8q'
        Mark "S8q desktopProcessesLeft=$(Desktop-Processes)"
    },
    { Terminal-First 'S9' },          # 9
    { Finish 'S10' 'RUNBOOK' }        # 10
)

# ---------------------------------------------------------------------------
# The steps the first run left out. All of them need Claude Desktop to be quit once.

$laterSteps = @(
    { # 0
        Preflight 'LATER' @(
            'CRP-002, the remaining steps. About 10 minutes.',
            'NOTE: part 4 quits Claude Desktop. Every Claude Code session running inside the app is interrupted',
            'and can be resumed afterwards. Only start when that is fine.')
    },
    { # 1
        Install-Prompt 'L1'
        Pause-For 10 'letting the servers start'; Servers 'L1'
        Step 'L1b' 'Settings > Extensions > "CRP-002 spike (throwaway)" > Configure. Turn ON "Linger after the app closes the connection". Save.'
        Pause-For 10 'watching for a restart'; Servers 'L1b'
    },
    { # 2
        Step 'L2' 'Go to the CHAT tab, not Code and not Cowork. Start a new chat and send:  Please call the spike_status tool and tell me what it returns.   Approve the tool if Claude asks.'
        Ask 'L2' 'Did Claude call the tool and show a result? Was there a permission prompt? (few words)' | Out-Null
        Servers 'L2'
        Step 'L2b' 'If Claude Desktop lets you open a second window (look in the File menu), open one. If it does not, just press Enter.'
        Ask 'L2b' 'Did a second window open? (y/n)' | Out-Null
        Pause-For 6 'settling'; Servers 'L2b'
    },
    { # 3
        Step 'L3a' 'Close every Claude Desktop window with the X button. Do not quit from the tray.'
        Ask 'L3a' 'Is Claude still in the system tray? (y/n)' | Out-Null
        Pause-For 20 'watching the servers'; Servers 'L3a'
        Step 'L3b' 'Open the Claude Desktop window again (tray icon or Start menu).'
        Pause-For 10 'settling'; Servers 'L3b'
    },
    { # 4
        Say ''
        Say '      The next step quits Claude Desktop. Claude Code sessions inside it are interrupted; resume them afterwards.'
        Step 'L4' 'Quit Claude Desktop completely: right-click its tray icon and choose Quit (or File > Exit).'
        Watch-Quit
        Servers 'L4'
        Mark "L4 desktopProcessesLeft=$(Desktop-Processes)"
    },
    { Terminal-First 'L5' },          # 5
    { Finish 'L6' 'LATER' }           # 6
)

$list = $steps
if ($Later) { $list = $laterSteps }
for ($n = $StartAt; $n -lt $list.Count; $n++) {
    Write-Host ''
    Write-Host "===== part $n of $($list.Count - 1) =====" -ForegroundColor Yellow
    & $list[$n]
}
