---
title: 'mail_cli'
has_children: true
---

# mail\_cli

Gmail / JMAP Spam Checker CLI tool

## Commands

| Command | Description |
|---------|-------------|
| [scan](scan.md) | Scan folders matching label prefix for spam. |
| [search](search.md) | Search email history or export to mbox. |
| [write](write.md) | Write a message in your editor, then review. |
| [tui](tui.md) | Open interactive terminal email browser. |
| [watch](watch.md) | Watch accounts and notify on new mail. |
| [msg](msg.md) | Read, send, and manage messages. |
| [label](label.md) | Manage labels: download, upload, sort, move. |
| [mbox](mbox.md) | Inspect, validate and merge local mbox files. |
| [spam](spam.md) | Manage Spam folder and train filters. |
| [rule](rule.md) | Manage routing rules and server filters. |
| [calendar](calendar.md) | Manage calendar events from attachments. |
| [contacts](contacts.md) | Manage contacts and aliases. |
| [account](account.md) | Configure accounts and sign in. |
| [config](config.md) | Show or manage configuration options. |
| [utils](utils.md) | Maintain cache and shell integration. |
| [docs](docs.md) | Generate Markdown help pages |

## Global Flags

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

## Examples

- `mail_cli %` — Scan the inbox -- a label is written %name, and % alone is the inbox
- `mail_cli %work:receipts` — Scan one folder on one account
- `mail_cli tui` — Browse mail interactively
- `mail_cli label sort %receipts` — File a folder into the dated keep/ hierarchy
- `mail_cli spam sweep` — Move spam out of the inbox and train the filter
- `mail_cli -E <command>` — Show the examples for one command

## Version

0.7.49

## Config file location

`~/.config/mail_cli/config.json`

## About

GitHub: [sarielhp/mail_cli](https://github.com/sarielhp/mail_cli)

