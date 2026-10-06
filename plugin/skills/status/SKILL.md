---
name: status
description: Check whether Discord Rich Presence is working in this session. Use when the user asks whether their Discord status is showing, why it is not, or what the presence server is doing.
---

# Presence status

Call the tool `presence_status` of this plugin's `presence` server. Its full name is `mcp__plugin_rich-presence_presence__presence_status`. Call it with no arguments. It reads nothing from the session and changes nothing. Claude Code may ask the user for permission the first time.

Then tell the user what it reported, in plain language and in a few sentences:

- **Role.** `host` means this session holds the connection to Discord. `follower` means another session or app holds it and this one reports to it. Both are normal. `off` means presence is switched off in the configuration.
- **Discord.** `connected` means the status is being shown. `connecting` or `disconnected` means the Discord desktop app was not found: ask the user to check that Discord is running on this machine, and say that presence returns by itself once it is.
- **Sessions.** How many open sessions are being shown as one status. A session whose project is hidden is not counted.
- **Privacy.** The level this session is published at. `off` means this session is hidden: its project's privacy level is `off`, so it is not published at all.
- **Paused.** `no` is the usual state. Otherwise presence is paused for every session, until resumed or for the time given, and the `resume` skill ends the pause. `unavailable` means another open session holds the Discord connection and runs an older version that cannot pause.

If the tool is not available, the server did not start. Say so, and suggest restarting the session. The server is downloaded the first time the plugin loads, so a failed download leaves the plugin enabled with no server.

What the tool reports this way names no project and is safe to paste anywhere. To see what the card itself says, use the `preview` skill, whose result is private.

Do not guess at anything the tool did not report.
