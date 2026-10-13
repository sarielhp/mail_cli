---
title: 'mail_cli label cache update'
parent: 'mail_cli label cache'
---

# mail\_cli label cache update

Fetch the latest labels and folder hierarchies from the server and refresh the local cache.

## Usage

```
mail_cli label cache update
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

- `mail_cli label cache update`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
