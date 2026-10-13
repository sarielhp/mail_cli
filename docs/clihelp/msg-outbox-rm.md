---
title: 'mail_cli msg outbox rm'
parent: 'mail_cli msg outbox'
---

# mail\_cli msg outbox rm

Delete queued messages from outbox.

## Usage

```
mail_cli msg outbox rm <id>...
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

---

[↑ mail\_cli](index.md) — [nav](nav.md)
