---
title: 'mail_cli label move'
parent: 'mail_cli label'
---

# mail\_cli label move

Move emails matching pattern to target label.

## Usage

```
mail_cli label move <source_label> <pattern> <target_label> [--do]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<source_label>` | Source folder/label containing messages to split. |
| `<pattern>` | Subject pattern with wildcards (*, ?) to match. |
| `<target_label>` | Destination folder/label to move matched messages to. |

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
| `--do` | Perform the actual move operations |

## Examples

- `mail_cli label move Inbox "*invoice*" Receipts`
- `mail_cli label move Inbox "*invoice*" Receipts --do`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
