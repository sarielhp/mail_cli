---
title: 'mail_cli calendar add-all'
parent: 'mail_cli calendar'
---

# mail\_cli calendar add-all

Scan inbox for .ics attachments and add them to calendar.

## Usage

```
mail_cli calendar add-all
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

- `mail_cli calendar add-all`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
