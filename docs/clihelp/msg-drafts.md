---
title: 'mail_cli msg drafts'
has_children: true
parent: 'mail_cli msg'
---

# mail\_cli msg drafts

Every compose session keeps a local copy of the message under the cache's drafts directory, rewritten on each edit and on exit, beside the copy it uploads to the account's Drafts folder. The listing names each local copy by an ID that 'write --resume <id>' reopens on the review screen. Sending a message removes its local copy; leaving with q or Esc keeps it, and 'rm' deletes it. The 'server' subcommand is the counterpart for the account's Drafts folder: it lists, trashes and prunes the copies uploaded there.

## Usage

```
mail_cli msg drafts <ls | rm <id>... | server ...>
```

## Subcommands

| Command | Description |
|---------|-------------|
| [ls](msg-drafts-ls.md) | List saved drafts, newest first. |
| [rm](msg-drafts-rm.md) | Delete saved drafts by ID. |
| [server](msg-drafts-server.md) | List, trash or prune drafts on the server. |

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

- `mail_cli msg drafts ls` — List saved drafts, newest first
- `mail_cli write --resume 2026/09/30/125` — Reopen a draft on the review screen
- `mail_cli msg drafts rm 2026/09/30/125` — Delete a draft
- `mail_cli msg drafts server prune --sent` — Trash duplicate and already-sent server drafts

---

[↑ mail\_cli](index.md) — [nav](nav.md)
