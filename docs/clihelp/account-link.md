---
title: 'mail_cli account link'
parent: 'mail_cli account'
---

# mail\_cli account link

Associate a program/symlink name with an account for automatic account selection.

## Usage

```
mail_cli account link <account_name> <program_name>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<account_name>` | The existing account name to associate with. |
| `<program_name>` | Program name that selects this account. |

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

- `mail_cli account link work-account work_mail`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
