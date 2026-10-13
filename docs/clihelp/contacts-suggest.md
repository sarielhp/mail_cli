---
title: 'mail_cli contacts suggest'
parent: 'mail_cli contacts'
---

# mail\_cli contacts suggest

Suggest ranked email recipients combining curated contacts and interaction history.

## Usage

```
mail_cli contacts suggest [query]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[query]` | Optional prefix or search string to match recipients. |

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

- `mail_cli contacts suggest`
- `mail_cli contacts suggest alice`
- `mail_cli contacts suggest algo`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
