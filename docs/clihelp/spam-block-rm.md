---
title: 'mail_cli spam block rm'
parent: 'mail_cli spam block'
---

# mail\_cli spam block rm

Remove email addresses from blocklist.

## Usage

```
mail_cli spam block rm <email...>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<email...>` | Email addresses to remove from blacklist. |

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

- `mail_cli spam block rm spammer@example.com`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
