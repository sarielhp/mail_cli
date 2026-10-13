---
title: 'mail_cli account user-name'
parent: 'mail_cli account'
---

# mail\_cli account user-name

Set the personal name placed in the From header of mail you send from an account. It is separate from the account's display name, which only labels the account in listings and the TUI; the display name is never used as the sender.

## Usage

```
mail_cli account user-name <account_name> <user_name>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<account_name>` | The account whose outgoing name to set. |
| `<user_name>` | Name to show in the From header, e.g. "Sariel Har-Peled". |

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

- `mail_cli account user-name fastmail "Sariel Har-Peled"`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
