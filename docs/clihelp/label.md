---
title: 'mail_cli label'
has_children: true
---

# mail\_cli label

Subcommands include download (a folder to mbox) and upload (an mbox into a folder), sort and move (file mail into the keep/ hierarchy). Manage, inspect, and organize remote mail server labels and hierarchical folder trees across accounts, including cached folder statistics and structure repair.

## Usage

```
mail_cli label <subcommand> [args...]
```

## Subcommands

| Command | Description |
|---------|-------------|
| [list (ls)](label-list.md) | List all folders and labels in a tree view. |
| [print (pr)](label-print.md) | Print all labels, one per line. |
| [rename](label-rename.md) | Rename a label and update rules. |
| [repair (fix)](label-repair.md) | Ensure parent folders exist for nested paths. |
| [rm (del, delete)](label-rm.md) | Delete a label or folder from the server. |
| [search](label-search.md) | Search cached labels by pattern. |
| [cache](label-cache.md) | Manage the local labels cache. |
| [create](label-create.md) | Create a label or folder on the server. |
| [count](label-count.md) | Count the messages in a label or folder. |
| [archive](label-archive.md) | Archive every message in a folder. |
| [download](label-download.md) | Download messages to local mbox file(s). |
| [upload](label-upload.md) | Upload messages from mbox file to folder. |
| [sort](label-sort.md) | Organize messages into dated keep/ hierarchy. |
| [move](label-move.md) | Move emails matching pattern to target label. |

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

- `mail_cli label list` — List folders that hold messages
- `mail_cli label list --ext` — Include message counts and hierarchy
- `mail_cli label count %inbox` — How many messages are in the inbox
- `mail_cli label count %inbox -q` — Just the number, for a script
- `mail_cli label search receipts` — Find folders whose name contains a word
- `mail_cli label create "%Receipts/2026"` — Create a nested folder
- `mail_cli label rm "%Receipts/2025"` — Delete a folder
- `mail_cli label rename "%OldName" "%NewName"` — Rename a folder in place
- `mail_cli label repair` — Create the missing parents of nested folders

---

[↑ mail\_cli](index.md) — [nav](nav.md)
