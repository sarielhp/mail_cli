---
title: 'mail_cli rule rm-all'
parent: 'mail_cli rule'
---

# mail\_cli rule rm-all

Delete all routing rules for the selected account.

## Usage

```
mail_cli rule rm-all [--yes]
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
| `-y, --yes` | Proceed without the confirmation prompt. |

## Examples

- `mail_cli rule rm-all`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
