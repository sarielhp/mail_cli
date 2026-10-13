---
title: 'mail_cli spam block add'
parent: 'mail_cli spam block'
---

# mail\_cli spam block add

Add email addresses to blocklist.

## Usage

```
mail_cli spam block add <email...>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<email...>` | Email addresses to add to blacklist. |

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
| `--trash` | Route to Trash instead of Spam (no spam filter training) |
| `--push` | Immediately push updated rules to the server |

## Examples

- `mail_cli spam block add spammer@example.com`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
