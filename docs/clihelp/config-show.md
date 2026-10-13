---
title: 'mail_cli config show'
parent: 'mail_cli config'
---

# mail\_cli config show

Display the active configuration values.

## Usage

```
mail_cli config show
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

- `mail_cli config show`
- `mail_cli config`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
