---
title: 'mail_cli spam block'
has_children: true
parent: 'mail_cli spam'
---

# mail\_cli spam block

Manage the personal sender blacklist to instantly classify messages as spam.

## Usage

```
mail_cli spam block <subcommand> [args...]
```

## Subcommands

| Command | Description |
|---------|-------------|
| [add](spam-block-add.md) | Add email addresses to blocklist. |
| [rm (del)](spam-block-rm.md) | Remove email addresses from blocklist. |
| [list](spam-block-list.md) | List blocked email addresses. |

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

- `mail_cli spam block list`
- `mail_cli spam block add spammer@example.com`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
