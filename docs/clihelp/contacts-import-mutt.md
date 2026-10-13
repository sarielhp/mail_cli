---
title: 'mail_cli contacts import-mutt'
parent: 'mail_cli contacts'
---

# mail\_cli contacts import-mutt

Import contacts and distribution lists from a Mutt alias file.

## Usage

```
mail_cli contacts import-mutt [file_path]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[file_path]` | Mutt alias file (default: from config) |

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

- `mail_cli contacts import-mutt`
- `mail_cli contacts import-mutt ~/.mutt/aliases`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
