---
title: 'mail_cli rule list'
parent: 'mail_cli rule'
---

# mail\_cli rule list

List custom routing rules with their target labels and export status.

## Usage

```
mail_cli rule list [flags]
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
| `-a, --all` | List all custom routing rules, including those already exported |

## Examples

- `mail_cli rule list`
- `mail_cli rule list --all`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
