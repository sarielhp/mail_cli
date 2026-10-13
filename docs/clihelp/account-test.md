---
title: 'mail_cli account test'
parent: 'mail_cli account'
---

# mail\_cli account test

Check an account's config and connection.

## Usage

```
mail_cli account test [account_name]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[account_name]` | Account name to test (defaults to currently selected account). |

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

- `mail_cli account test`
- `mail_cli account test work-fastmail`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
