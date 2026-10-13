---
title: 'mail_cli label archive'
parent: 'mail_cli label'
---

# mail\_cli label archive

Archive every message in a folder.

## Usage

```
mail_cli label archive [%label]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[%label]` | The folder to empty into the archive. Defaults to the inbox. |

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

- `mail_cli label archive` — Archive everything in the inbox
- `mail_cli label archive %receipts` — Archive everything in one folder

---

[↑ mail\_cli](index.md) — [nav](nav.md)
