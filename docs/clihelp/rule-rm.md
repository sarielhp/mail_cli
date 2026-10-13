---
title: 'mail_cli rule rm'
parent: 'mail_cli rule'
---

# mail\_cli rule rm

Remove an auto-labeling rule by sender email address or subject title.

## Usage

```
mail_cli rule rm <email|title>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<email\|title>` | The sender email or subject title to remove rule for. |

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

- `mail_cli rule rm notifications@github.com`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
