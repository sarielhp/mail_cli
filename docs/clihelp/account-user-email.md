---
title: 'mail_cli account user-email'
parent: 'mail_cli account'
---

# mail\_cli account user-email

Set the From address used when sending from an account, which may differ from the login username -- a Fastmail alias or a verified send-as identity, for example. The login credential is unchanged; only the From header and the submission envelope use this address.

## Usage

```
mail_cli account user-email <account_name> <email>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<account_name>` | The account whose send address to set. |
| `<email>` | Address to send from, e.g. an alias of the account. |

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

- `mail_cli account user-email fastmail sariel@sarielhp.org`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
