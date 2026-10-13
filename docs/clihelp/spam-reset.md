---
title: 'mail_cli spam reset'
parent: 'mail_cli spam'
---

# mail\_cli spam reset

Discard every local spam classification and score for this account.

## Usage

```
mail_cli spam reset
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

- `mail_cli spam reset` — Forget what the filter learned locally and re-classify

---

[↑ mail\_cli](index.md) — [nav](nav.md)
