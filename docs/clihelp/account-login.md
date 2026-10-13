---
title: 'mail_cli account login'
parent: 'mail_cli account'
---

# mail\_cli account login

Perform interactive OAuth login for a Gmail or Outlook account.

## Usage

```
mail_cli account login [account_name]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[account_name]` | Account name to re-authenticate (defaults to active account). |

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

- `mail_cli account login`
- `mail_cli account login personal-gmail`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
