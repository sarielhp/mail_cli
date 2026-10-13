---
title: 'mail_cli spam pol audit'
parent: 'mail_cli spam pol'
---

# mail\_cli spam pol audit

Scan Spam folder for political fundraising emails and print scoring details.

## Usage

```
mail_cli spam pol audit
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

## Examples

- `mail_cli spam pol audit` — Report political mail in Spam, changing nothing

---

[↑ mail\_cli](index.md) — [nav](nav.md)
