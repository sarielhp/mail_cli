---
title: 'mail_cli account calendar'
parent: 'mail_cli account'
---

# mail\_cli account calendar

Show or set the calendar manager account.

## Usage

```
mail_cli account calendar [account_name]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[account_name]` | Account name to configure calendar for. |

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

- `mail_cli account calendar`
- `mail_cli account calendar personal-gmail`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
