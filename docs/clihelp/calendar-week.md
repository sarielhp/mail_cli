---
title: 'mail_cli calendar week'
parent: 'mail_cli calendar'
---

# mail\_cli calendar week

Show all calendar events in the upcoming week or specified number of days.

## Usage

```
mail_cli calendar week [days]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[days]` | Number of days to display (defaults to 7). |

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

- `mail_cli calendar week`
- `mail_cli calendar week 14`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
