---
title: 'mail_cli mbox info'
parent: 'mail_cli mbox'
---

# mail\_cli mbox info

Display message count, total file size, and attachment sizes for an mbox file.

## Usage

```
mail_cli mbox info <file>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<file>` | Mbox file to inspect |

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

- `mail_cli mbox info archive.mbox`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
