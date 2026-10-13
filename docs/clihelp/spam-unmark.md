---
title: 'mail_cli spam unmark'
has_children: true
parent: 'mail_cli spam'
---

# mail\_cli spam unmark

Mark a message as not being spam: train Bogofilter as ham and move it from Spam back to Inbox on the server.

## Usage

```
mail_cli spam unmark <message_id...>
  mail_cli unspam folder <folder_name>
```

## Subcommands

| Command | Description |
|---------|-------------|
| [folder](spam-unmark-folder.md) | Mark all messages in folder as ham. |

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<message_id...>` | One or more message IDs to unspam (short 8-char or full). |

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

- `mail_cli spam unmark abc123de`
- `mail_cli spam unmark folder %spam`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
