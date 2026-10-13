---
title: 'mail_cli contacts list'
parent: 'mail_cli contacts'
---

# mail\_cli contacts list

List stored contacts and aliases with an optional substring filter.

## Usage

```
mail_cli contacts list [filter]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[filter]` | Optional substring to filter by name, email, or alias. |

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

- `mail_cli contacts list`
- `mail_cli contacts list john`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
