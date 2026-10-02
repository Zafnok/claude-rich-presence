# Risk register

Ordered by how much damage each could do to the plan. "Retired by" names the ticket whose outcome settles it.

| # | Risk | Likelihood | Impact | Mitigation | Retired by |
|---|---|---|---|---|---|
| R1 | The preferred Claude Code wiring, an MCPB bundle plus `mcp_tool` hooks, does not behave as documented. Nobody has shipped it | Medium | High: changes the M4 tickets | A specified fallback that is known to work. The core is unaffected either way. Spike runs first | CRP-001 |
| R2 | A synchronous hook call slows Claude | Low | High: violates the first design rule | The tool does no I/O, every hook has a two-second timeout, and the end-to-end tests enforce a latency budget | CRP-001, CRP-043 |
| R3 | Claude Desktop does not keep an extension's server alive for the app's lifetime, or starts one per conversation | Medium | Medium: Desktop presence would need redesign or be dropped | Spike before any Desktop work. Desktop support is partial by design, so the worst case is a smaller feature | CRP-002 |
| R4 | On Windows, Claude Desktop is a packaged app. Its child processes may see a redirected data directory, so the lock and socket would not be shared with terminal sessions | Medium | Medium: two hosts, duplicated activity | Spike checks it. Remedies: a location that is not redirected, or a named pipe for the control channel | CRP-002 |
| R5 | Unsigned binaries are blocked or warned about: Gatekeeper on macOS if the bundle carries a quarantine attribute, antivirus heuristics on Windows | Medium | Medium: installation friction | Test during the spikes. Free signing for open source may be available. Apple notarisation costs money and is an owner decision | CRP-002, CRP-063 |
| R6 | Discord's name filter or developer policy blocks the application name, or Anthropic objects to naming | Medium | Medium | Neutral names, original artwork, no filter evasion, an unaffiliated notice, a configurable application id | CRP-003 |
| R7 | Discord changes or restricts the local IPC protocol. Setting an activity without OAuth is how Discord's own library behaves, but the documentation does not promise it | Low | High: no presence at all | The protocol is isolated in one package. The `discord-ipc` skill lists what to re-verify | Ongoing |
| R8 | Hook events, the plugin manifest or MCPB change | High over a year | Low to medium | Adapters are thin. Plugin validation runs in CI. The `claude-surfaces` skill lists what to re-verify | Ongoing |
| R9 | The model name is missing for most of a short session | High under the preferred wiring | Low: cosmetic | Shown once known. A follow-up spike looks for a documented source | CRP-045 |
| R10 | Election or failover has a race that leaves no host, or two | Medium | Medium | An operating-system lock, not a protocol, decides the host. Strongest model and effort on that ticket. Failover scenarios in end-to-end tests with the race detector | CRP-032, CRP-043 |
| R11 | 100% coverage is met by tests that assert nothing | Medium | Medium: false assurance | Review rule, fuzzing, race detector, each side of each condition tested | Ongoing, CRP-005 |
| R12 | Named-pipe I/O through the standard library misbehaves under concurrent read and write | Low | Medium | Overlapped I/O in Go 1.26. A Microsoft-maintained package is pre-cleared as the fallback | CRP-021 |
| R13 | The fine-grained status is invisible in practice because updates are limited to one per 15 seconds | High | Low | Design for it: the scheduler always sends the latest state, and status vocabulary is coarse | CRP-013 |
| R14 | SonarQube Cloud's free plan does not allow a custom quality gate | Medium | Low | The binding coverage gate is our own check in CI | CRP-006 |
| R15 | The plugin's URL-referenced bundle is fetched over HTTPS with no checksum field | Low | Medium | Same trust root as the repository itself. Releases carry checksums and a provenance attestation for anyone who wants to verify | CRP-060 |
| R16 | The activity summary publishes something the user did not want public, or untrusted content in a project steers what it says | Medium for opted-in users | Medium | Opt-in only, limited to chosen project directories, sanitised to one short line of plain text with no links or mentions, never logged. Documented as model-written | CRP-046, CRP-047 |
| R17 | Claude does not call the summary tool reliably, or a permission prompt interrupts the user | Medium | Low: the feature is dropped or marked experimental | Spike with explicit pass criteria before any code. A hook-delivered reminder as the fallback nudge | CRP-046 |
| R18 | A published repository link points at a private repository, or a repository is made private later | Low | Low: the owner and name are revealed and the link leads nowhere | Per-project opt-in only. The setup skill checks visibility with the user's GitHub CLI. Documented | CRP-048 |
| R19 | In a permission mode without prompts, text in a repository talks Claude into opting that project in by editing the configuration file | Low | Low: a link and phrase for that project are published | No tool can write profiles. The edit is visible and prompted in default modes. The link is validated. Documented | CRP-048, CRP-062 |
| R20 | Sessions in one project label related work with different area names, so the shared line does not appear | Medium without a list of areas, low with one | Low: presence shows the focus session's own phrase instead | Areas listed in the project profile. Agreement measured in the spike before the roll-up is built | CRP-046, CRP-049 |

## Accepted limitations

These are not risks. They are known and will not be fixed without a change on a vendor's side.

- Claude Desktop Chat shows only that the app is open, unless the opt-in summary of ADR-0011 is accepted and enabled.
- No presence for cloud sessions, the web, mobile, or setups where Discord runs on a different operating system instance than Claude, including WSL and remote development.
- Linux on Arm is not in the bundle.
- A failover between hosts causes a brief gap in presence.
