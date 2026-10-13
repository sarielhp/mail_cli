---
title: 'mail_cli msg'
has_children: true
---

# mail\_cli msg

Use msg show to read a message or folder, msg send for scripts and pipes, and msg reply to continue a thread. msg drafts manages saved work, msg last lists recent mail, and msg export saves messages or threads. Write new mail with write. Archive and block operate on message IDs; label archive acts on a whole folder.

## Usage

```
mail_cli msg <subcommand> [args...]
```

## Subcommands

| Command | Description |
|---------|-------------|
| [show](msg-show.md) | Show emails by label prefix or message ID. |
| [send](msg-send.md) | Send an email directly or from stdin. |
| [reply](msg-reply.md) | Reply to an email preserving thread headers. |
| [drafts](msg-drafts.md) | List or remove locally saved drafts. |
| [last](msg-last.md) | Show recent emails across all folders. |
| [export](msg-export.md) | Export emails or threads to mbox or eml. |
| [pipe](msg-pipe.md) | Pipe message raw RFC822 or body to command. |
| [outbox](msg-outbox.md) | Manage offline outbound email queue. |
| [archive](msg-archive.md) | Archive one or more messages by ID. |
| [block](msg-block.md) | Block the senders of messages and move them. |

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

- `mail_cli msg show abc123de` — Read a message
- `mail_cli msg last 20` — List recent messages across folders
- `mail_cli msg archive abc123de` — Archive one message by ID
- `mail_cli msg block abc123de` — Block sender of message to Spam

---

[↑ mail\_cli](index.md) — [nav](nav.md)
