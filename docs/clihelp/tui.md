---
title: 'mail_cli tui'
has_children: true
---

# mail\_cli tui

Open the interactive terminal email browser. With an optional label_prefix argument, open the TUI with the matching label as the initial folder. The prefix is matched case-insensitively as a substring against the full label path. If exactly one label matches, the TUI opens on that label. If multiple match, all matching labels are printed and the program exits. Given an existing .eml file instead, open that one message read-only, as mbox view does. Use tui write to compose a message; it accepts the same arguments and flags as write, including --resume.

## Usage

```
mail_cli tui [label_prefix]
  mail_cli tui <file.eml>
  mail_cli tui write [flags] [recipient...]
```

## Subcommands

| Command | Description |
|---------|-------------|
| [write](tui-write.md) | Write a message in your editor, then review. |

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[label_prefix]` | Folder/label prefix to open into, or a .eml file to view. |

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

- `mail_cli tui`
- `mail_cli tui %receipts`
- `mail_cli tui saved.eml` — View one saved message, read-only
- `mail_cli tui write boss@example.com -s Update` — Compose through a wrapper that opens the TUI

---

[↑ mail\_cli](index.md) — [nav](nav.md)
