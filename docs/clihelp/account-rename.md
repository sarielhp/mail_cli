---
title: 'mail_cli account rename'
parent: 'mail_cli account'
---

# mail\_cli account rename

Rename an existing account's display name.

## Usage

```
mail_cli account rename <old_name> <new_name>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<old_name>` | The current name of the account to rename. |
| `<new_name>` | The new name for the account. |

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

- `mail_cli account rename old-gmail new-gmail`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
