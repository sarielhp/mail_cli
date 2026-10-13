---
title: 'mail_cli mbox check'
parent: 'mail_cli mbox'
---

# mail\_cli mbox check

Check if a file (or all files in a directory) is a valid mbox file.

## Usage

```
mail_cli mbox check <file>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<file>` | Mbox file or directory to validate |

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

- `mail_cli mbox check inbox.mbox`
- `mail_cli mbox check ~/archive/`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
