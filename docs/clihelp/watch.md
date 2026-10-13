---
title: 'mail_cli watch'
---

# mail\_cli watch

Poll every account in the background, notify on messages that arrived since the last poll, run a script per message, and flush the offline outbox. It runs in the foreground until interrupted; put it under systemd --user or in a tmux window to keep it alive. The first poll of an account records what is already there without notifying.

## Usage

```
mail_cli watch [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| `-A, --account <name>` | Target account name |
| `--account-index N` | Target account by config index (also -N) |
| `--read-only` | Block server writes (cache still updates) |
| `--dry-run` | Report changes only; implies --read-only |
| `-v, --verbose` | Enable verbose output |
| `--json` | Emit result as JSON on stdout |
| `--logappend` | Append to existing logs instead of truncating |
| `--version` | Print the version number and exit |
| `-i, --interval <duration>` | How often to poll (minimum 30s) (default: 2m0s) |
| `--on-mail <path>` | Run this program once per new message |
| `--no-notify` | Do not send desktop notifications |
| `--no-flush` | Do not flush the offline outbox |

## Examples

- `mail_cli watch` — Watch every account, notifying on new mail
- `mail_cli watch -A work --no-notify` — Watch one account, log instead of notifying
- `mail_cli watch --on-mail ~/bin/on-mail.sh` — Run a script for each new message

## Which accounts

Every account is watched by default; pass the global -A/--account to watch just one.

## Notifications

New mail is announced as a desktop notification when notify-send is available, and as a log line otherwise.

## The --on-mail hook

--on-mail passes the message in the environment: MAIL_CLI_ACCOUNT, MAIL_CLI_FROM, MAIL_CLI_SUBJECT, MAIL_CLI_DATE, MAIL_CLI_ID. A hook still running after 30 seconds is killed.

## The first poll

The first poll of an account records existing messages as seen and announces nothing, so starting the watcher does not notify you about the whole inbox.

## What is announced

Mail in spam, trash, sent and drafts folders is never announced, nor is mail dated more than six hours before the previous poll: an older message that came into view because newer ones were deleted or moved.

## Failures

After a failed poll it waits longer each time, up to ten minutes, and reports the failure instead of exiting.

---

[↑ mail\_cli](index.md) — [nav](nav.md)
