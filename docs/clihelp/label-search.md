---
title: 'mail_cli label search'
parent: 'mail_cli label'
---

# mail\_cli label search

Search cached labels by one or more substring patterns (all must match).

## Usage

```
mail_cli label search <substring> [additional_substrings...]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<substring>` | Substring to search for in folder paths. |
| `[additional_substrings...]` | Extra substrings; all must match. |

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

- `mail_cli label search receipts`
- `mail_cli label search archive 2026`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
