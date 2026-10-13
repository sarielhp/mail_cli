---
title: 'mail_cli spam allow list'
parent: 'mail_cli spam allow'
---

# mail\_cli spam allow list

List allowed email addresses.

## Usage

```
mail_cli spam allow list
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

- `mail_cli spam allow list`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
