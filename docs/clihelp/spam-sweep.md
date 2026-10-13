---
title: 'mail_cli spam sweep'
parent: 'mail_cli spam'
---

# mail\_cli spam sweep

Execute a complete sweep: unsubscribe from political lists, train spam filters on all messages in Spam, and purge the Spam folder.

## Usage

```
mail_cli spam sweep
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

- `mail_cli spam sweep` — Move spam out of the inbox and train the filter

---

[↑ mail\_cli](index.md) — [nav](nav.md)
