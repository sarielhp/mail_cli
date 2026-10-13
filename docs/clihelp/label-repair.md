---
title: 'mail_cli label repair'
parent: 'mail_cli label'
---

# mail\_cli label repair

Ensure that all parent folders exist for every nested folder path in the account.

## Usage

```
mail_cli label repair
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

- `mail_cli label fix`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
