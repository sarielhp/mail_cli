---
title: 'mail_cli utils cache prune'
parent: 'mail_cli utils cache'
---

# mail\_cli utils cache prune

Prune cached emails and spam scores older than the specified number of days (default: 30). With --wipe, purges all cached messages regardless of age.

## Usage

```
mail_cli utils cache prune [days] [-w, --wipe]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[days]` | Age threshold in days for pruning cached emails (default: 30). |

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
| `-w, --wipe` | Purge all cached email files regardless of age. |

## Examples

- `mail_cli utils cache prune`
- `mail_cli utils cache prune 7`
- `mail_cli utils cache prune --wipe`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
