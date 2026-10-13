---
title: 'mail_cli spam forget'
parent: 'mail_cli spam'
---

# mail\_cli spam forget

Drop your spam/not-spam decision on messages, made by mark, unmark or learn --ham, so the next scan judges them. Training is kept and nothing changes on the server; a %folder reference covers every message the cache lists in that folder.

## Usage

```
mail_cli spam forget <message_id...|%folder>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<message_id...\|%folder>` | Message IDs, or one folder reference. |

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

- `mail_cli spam forget abc123de` — Let the filter decide about this one again
- `mail_cli spam forget %received` — Undo a learn --ham on a whole folder

---

[↑ mail\_cli](index.md) — [nav](nav.md)
