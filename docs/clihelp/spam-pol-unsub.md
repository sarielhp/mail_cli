---
title: 'mail_cli spam pol unsub'
parent: 'mail_cli spam pol'
---

# mail\_cli spam pol unsub

Scan the Spam folder for political messages and execute unsubscription opt-outs.

## Usage

```
mail_cli spam pol unsub
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

- `mail_cli spam pol unsub` — Send opt-outs and delete what was unsubscribed from

---

[↑ mail\_cli](index.md) — [nav](nav.md)
