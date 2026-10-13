---
title: 'mail_cli account rm'
parent: 'mail_cli account'
---

# mail\_cli account rm

Delete an account and its credentials.

## Usage

```
mail_cli account rm <account_name>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<account_name>` | The name of the account to delete. |

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

- `mail_cli account rm temp-account`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
