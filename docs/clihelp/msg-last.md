---
title: 'mail_cli msg last'
parent: 'mail_cli msg'
---

# mail\_cli msg last

Check all folders in the account, collect the last N emails received, save them to a virtual mailbox, and print each email with its folder name.

## Usage

```
mail_cli msg last <N>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<N>` | Number of most recent emails to retrieve and display. |

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

- `mail_cli msg last 10`
- `mail_cli msg last 25`
- `mail_cli -A work-jmap msg last 20`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
