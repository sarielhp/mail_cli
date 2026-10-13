---
title: 'mail_cli rule remote rm'
parent: 'mail_cli rule remote'
---

# mail\_cli rule remote rm

Delete a server filter and its local rule.

## Usage

```
mail_cli rule remote rm <target> [flags]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<target>` | The filter index, filter ID, or sender/subject to delete. |

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
| `-y, --yes` | Proceed without the confirmation prompt. |
| `--keep-local` | Keep matching local rule in config.json (mark as unexported). |

## Examples

- `mail_cli rule remote rm 1`
- `mail_cli rule remote rm AN63b9f`
- `mail_cli rule remote rm alerts@github.com`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
