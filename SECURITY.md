# Security policy

## Supported versions

Only the latest release is supported. A fix is made on `main` and published as a new release; older releases are not patched.

No release has been published yet. Until the first one, reports against `main` are welcome.

## Reporting a vulnerability

Report it privately, through GitHub: open the repository's [Security tab](https://github.com/Zafnok/claude-rich-presence/security) and choose **Report a vulnerability**. Only the maintainer sees the report.

Please do not open a public issue or pull request for something you believe is a vulnerability.

A useful report says:

- the version, from `rich-presence version`, and the operating system;
- what you did and what happened, as steps someone else can follow;
- what you think it lets an attacker do.

The output of `rich-presence doctor` is designed to be safe to share and often helps. Do not include prompts or file contents.

This is a project with one maintainer and no service-level promise. Expect an acknowledgement within a week. If the report is accepted, you will be told what the fix will be and when it is released, and credited in the release notes unless you ask not to be.

## What counts

The program runs as you, beside a coding agent, and publishes to your Discord profile. It makes no network connection of its own. What it must never do, and what is already known and accepted, is in [the threat model](docs/architecture/threat-model.md). In short, these are vulnerabilities:

- anything of your work, or anything your privacy level withholds, reaching Discord, another local user, the log or the model;
- another local user reading or steering your presence;
- input from a hook, the model, the control socket, Discord or the configuration that makes the program write a file, run something, open a network connection or stall Claude;
- a way to tamper with a release or the bundle that the published checksums and attestation would not show.

These are not:

- what a process already running as you can do to your own presence;
- a Discord client, or something pretending to be one, seeing the activity that was about to be published;
- anything that needs administrator or root access first.
