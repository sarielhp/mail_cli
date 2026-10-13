---
title: 'mail_cli label sort'
has_children: true
parent: 'mail_cli label'
---

# mail\_cli label sort

Organize and archive messages from a source folder/label into the date-based keep/YYYY/MM/<folder> structure (root "keep/" is fixed). Runs in dry-run mode by default; use --move to perform actual moves or --copy to copy without deleting from source. Use -f, -F, or -Y to customize destination folder naming.

## Usage

```
mail_cli label sort <folder> [flags]
mail_cli label sort examples
```

## Subcommands

| Command | Description |
|---------|-------------|
| [examples](label-sort-examples.md) | Show archive mapping patterns and examples. |

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<folder>` | Source folder/label name (e.g. "receipts" or "GMail:receipts"). |

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
| `--move` | Move the messages (default: dry run) |
| `--copy` | Copy the messages (default: dry run) |
| `-n <num>` | Number of messages to process (default: 10) (default: 10) |
| `-p, --pattern <pattern>` | Only messages whose subject has this |
| `-f, --folder <name>` | Folder keep/YYYY/MM/<name> |
| `-F, --folder-suffix <name>` | Folder keep/YYYY/MM/<name>-YYYY-MM |
| `-Y, --folder-year <name>` | Folder keep/YYYY/<name>-YYYY-MM |
| `--allow` | Allow sourcing from folders that start with keep/ |

## Examples

- `mail_cli label sort %receipts` — Dry run preview of date-based archive for receipts folder
- `mail_cli label sort %receipts --move` — Move receipts messages to keep/YYYY/MM/receipts
- `mail_cli label sort %receipts -f Archive --move` — Move receipts messages to keep/YYYY/MM/Archive
- `mail_cli label sort %receipts -F Receipts --move` — Move receipts messages to keep/YYYY/MM/Receipts-YYYY-MM
- `mail_cli label sort %GMail:received -Y %FastMail:services --move` — Cross-account move from GMail to FastMail keep/YYYY/services-YYYY-MM
- `mail_cli label sort %receipts -n 50 --copy` — Copy up to 50 receipts messages without deleting source
- `mail_cli label sort %receipts -p "*order*" --move` — Move only receipts messages matching pattern
- `mail_cli label sort examples` — Show detailed structured examples and mapping patterns

---

[↑ mail\_cli](index.md) — [nav](nav.md)
