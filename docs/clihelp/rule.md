---
title: 'mail_cli rule'
has_children: true
---

# mail\_cli rule

Configure local routing rules and sync them as remote server-side filters (e.g. Gmail filters or JMAP/Sieve scripts) to automatically label or discard incoming messages.

## Usage

```
mail_cli rule <subcommand> [args...]
```

## Subcommands

| Command | Description |
|---------|-------------|
| [add](rule-add.md) | Add rule by sender or message ID. |
| [add-title](rule-add-title.md) | Add rule matching subject prefix. |
| [add-domain](rule-add-domain.md) | Add rule for a sender's domain. |
| [rm (del)](rule-rm.md) | Remove rule by sender or subject. |
| [list (ls)](rule-list.md) | List custom routing rules. |
| [tidy](rule-tidy.md) | Remove duplicate routing rules. |
| [rm-all](rule-rm-all.md) | Delete all rules for account. |
| [update](rule-update.md) | Sync blacklisted senders to rules. |
| [push (export)](rule-push.md) | Export local rules to server filters. |
| [remote](rule-remote.md) | Manage remote server-side filters. |

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
| `--export <file>` | Export all existing rules to a file |
| `--import <file>` | Import rules from a file |

## Examples

- `mail_cli rule list`
- `mail_cli rule add notifications@github.com %Dev/GitHub`
- `mail_cli rule add-title "[ALERT]" "Alerts"`
- `mail_cli rule push`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
