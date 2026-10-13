---
title: 'mail_cli config unset'
parent: 'mail_cli config'
---

# mail\_cli config unset

Reset a configuration parameter to default.

## Usage

```
mail_cli config unset <key>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<key>` | The configuration parameter name to reset. |

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

- `mail_cli config unset score_threshold`
- `mail_cli config unset mutt_alias_file`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
