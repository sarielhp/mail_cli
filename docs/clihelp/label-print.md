---
title: 'mail_cli label print'
parent: 'mail_cli label'
---

# mail\_cli label print

Print all labels/folders, one per line (full path only).

## Usage

```
mail_cli label print
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

- `mail_cli label print`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
