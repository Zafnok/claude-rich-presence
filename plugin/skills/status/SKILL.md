---
name: status
description: Check whether Discord Rich Presence is working in this session. Use when the user asks whether their Discord status is showing, why it is not, or what the presence server is doing.
---

# Presence status

Call the tool `presence_status` of this plugin's `presence` server. Its full name is `mcp__plugin_rich-presence_presence__presence_status`. It takes no arguments, reads nothing from the session and changes nothing. Claude Code may ask the user for permission the first time.

Then tell the user what it reported, in plain language and in a few sentences:

- **Role.** `host` means this session holds the connection to Discord. `follower` means another session or app holds it and this one reports to it. Both are normal. `off` means presence is switched off in the configuration.
- **Discord.** `connected` means the status is being shown. `connecting` or `disconnected` means the Discord desktop app was not found: ask the user to check that Discord is running on this machine, and say that presence returns by itself once it is.
- **Sessions.** How many open sessions are being shown as one status.

If the tool is not available, the server did not start. Say so, and suggest restarting the session. The server is downloaded the first time the plugin loads, so a failed download leaves the plugin enabled with no server.

Do not guess at anything the tool did not report.
