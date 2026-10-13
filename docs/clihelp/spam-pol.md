---
title: 'mail_cli spam pol'
has_children: true
parent: 'mail_cli spam'
---

# mail\_cli spam pol

Inspect or unsubscribe from political fundraising emails in Spam. 'audit' inspects scoring details without mutating anything; 'unsub' sends opt-out requests and purges the emails.

## Usage

```
mail_cli spam pol <subcommand>
```

## Subcommands

| Command | Description |
|---------|-------------|
| [audit](spam-pol-audit.md) | Scan Spam for political mail scoring. |
| [unsub](spam-pol-unsub.md) | Scan Spam and execute opt-outs. |

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

## Examples

- `mail_cli spam pol audit` — Report political mail in Spam, changing nothing
- `mail_cli spam pol unsub` — Send opt-outs and delete what was unsubscribed from

---

[↑ mail\_cli](index.md) — [nav](nav.md)
