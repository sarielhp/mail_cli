---
title: 'mail_cli label upload'
parent: 'mail_cli label'
---

# mail\_cli label upload

Upload messages from mbox file to folder.

## Usage

```
mail_cli label upload <label> <file_name>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<label>` | The target label/folder to upload messages into. |
| `<file_name>` | The local mbox file path to read messages from. |

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

- `mail_cli label upload %receipts ~/archive/receipts.mbox`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
