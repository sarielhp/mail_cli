---
title: 'mail_cli label create'
parent: 'mail_cli label'
---

# mail\_cli label create

Create a label or folder on the server.

## Usage

```
mail_cli label create <lbl_name>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<lbl_name>` | The name or path of the label/folder to create. |

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

- `mail_cli labels create "Work/Projects"`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
