---
title: 'mail_cli rule remote list'
parent: 'mail_cli rule remote'
---

# mail\_cli rule remote list

List server-side filters for this account.

## Usage

```
mail_cli rule remote list
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

- `mail_cli rule remote list`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
