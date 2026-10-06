---
name: privacy
description: Explain what Discord Rich Presence shows about a session and how to change it. Use when the user asks what others can see in their Discord status, what the privacy levels mean, or how to show more or less.
---

# Presence privacy

Explain the levels and how to change the setting. Do not change any setting yourself.

## The levels

| Level | What the Discord status shows |
|---|---|
| `minimal` | That the assistant is in use, and nothing else |
| `standard` | Also the status, such as working, waiting or idle, and the model family. This is the default |
| `full` | Also the name of the project folder |
| `off` | Nothing. The session is hidden: it is not published and not counted |

At every level, prompts, replies, tool inputs and outputs, file contents and file paths are never read, stored or sent. The plugin's hooks name the few fields they pass, and nothing else leaves the session.

## Changing the level

1. Run `/config`, find the row **Privacy level** under the Rich Presence plugin, and pick `minimal`, `standard` or `full`. The same option is in `/plugin`, under the plugin's configuration.
2. Restart the session. The level is read when the presence server starts, so a session that is already open keeps the old one.

If several sessions are open, restart each of them.

## Hiding one project

The level can be set for one project, in the user's own configuration file, as a project profile. A project whose level is `off` is kept off Discord entirely, whatever the level elsewhere:

```json
{
  "projects": [
    { "path": "/home/me/work/client-project", "privacy": "off" }
  ]
}
```

The file is `%USERPROFILE%\.rich-presence\config.json` on Windows, `~/Library/Application Support/rich-presence/config.json` on macOS, and `~/.config/rich-presence/config.json` on Linux. The user edits it, and then restarts the sessions that are open.

## Switching presence off for a while

To stop showing anything for now, without changing a setting, use the `pause` skill. To see exactly what the card says at this moment, use the `preview` skill.
