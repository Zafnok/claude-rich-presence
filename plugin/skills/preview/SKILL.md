---
name: preview
description: Show the user exactly what their Discord Rich Presence card says right now, including the hover text and the link that they cannot see on their own profile. Use when the user asks what their Discord status shows, what others can see at this moment, or to preview their card.
---

# Preview the card

Call the tool `presence_status` of this plugin's `presence` server with the argument `preview` set to `true`. Its full name is `mcp__plugin_rich-presence_presence__presence_status`. It changes nothing. Claude Code may ask the user for permission the first time.

The result is **private**. It can name the user's project and its repository. Show it to the user in this conversation and nowhere else: do not write it to a file, a commit, an issue, a pull request or any other tool.

Show the user every part the tool reported, as a short list:

| Part | What it is on Discord |
|---|---|
| Line 1 and Line 2 | The two text lines of the card |
| Timer | The elapsed time counts up from this moment |
| Large image and its hover text | The main picture, and what appears when someone points at it |
| Small image and its hover text | The small badge on the picture, and what appears when someone points at it |
| Button and its link | A button other people see and can open. Discord does not show a user their own button, so this is the only place they can check it |

A part reported as `(empty)` is not shown on Discord. Say so plainly, do not leave it out.

Also tell the user what the first lines say:

- **This session.** Whether this session is published, and at which privacy level, or hidden because its project's level is `off`. A hidden session is not counted and never shown, but the card can still show the user's other sessions.
- **Presence.** Whether a card is shown at all. If it is paused, say for how much longer, and that the `resume` skill ends the pause. If nothing is shown, say why, as the tool reported it.

If the tool says the card is not known yet, call it once more. If it says the card cannot be read because of an older version, another open session holds the Discord connection: suggest restarting the other sessions so that they update.

Do not guess at anything the tool did not report.
