---
title: 'mail_cli config get'
parent: 'mail_cli config'
---

# mail\_cli config get

Print a configuration parameter's value.

## Usage

```
mail_cli config get <key>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<key>` | The configuration parameter name. |

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

- `mail_cli config get mutt_alias_file`
- `mail_cli config get browser`
- `mail_cli config get download_dir`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
