---
name: privacy
description: Explain what Discord Rich Presence shows about a session and how to change it. Use when the user asks what others can see in their Discord status, what the privacy levels mean, or how to show more or less.
---

# Presence privacy

Explain the three levels and how to change the setting. Do not change any setting yourself.

## The levels

| Level | What the Discord status shows |
|---|---|
| `minimal` | That the assistant is in use, and nothing else |
| `standard` | Also the status, such as working, waiting or idle, and the model family. This is the default |
| `full` | Also the name of the project folder |

At every level, prompts, replies, tool inputs and outputs, file contents and file paths are never read, stored or sent. The plugin's hooks name the few fields they pass, and nothing else leaves the session.

## Changing the level

1. Run `/config`, find the row **Privacy level** under the Rich Presence plugin, and pick `minimal`, `standard` or `full`. The same option is in `/plugin`, under the plugin's configuration.
2. Restart the session. The level is read when the presence server starts, so a session that is already open keeps the old one.

If several sessions are open, restart each of them.
