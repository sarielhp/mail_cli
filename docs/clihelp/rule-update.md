---
title: 'mail_cli rule update'
parent: 'mail_cli rule'
---

# mail\_cli rule update

Ensure all blacklisted senders have a corresponding local rule.

## Usage

```
mail_cli rule update
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

## Examples

- `mail_cli rule update`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
