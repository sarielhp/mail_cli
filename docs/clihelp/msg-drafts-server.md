---
title: 'mail_cli msg drafts server'
has_children: true
parent: 'mail_cli msg drafts'
---

# mail\_cli msg drafts server

Works on the selected account's Drafts folder. 'ls' shows each draft with the short ID 'rm' takes; a draft that cannot be read is listed as unreadable so it can still be removed. 'prune' trashes older copies of a message, recognised by a shared Message-ID, which compose gives every copy it uploads in one session. With --sent it also trashes drafts whose Message-ID is in the Sent folder.

## Usage

```
mail_cli msg drafts server <ls | rm <id>... | prune [--sent] [--by-subject]>
```

## Subcommands

| Command | Description |
|---------|-------------|
| [ls](msg-drafts-server-ls.md) | List the drafts on the server. |
| [rm](msg-drafts-server-rm.md) | Trash server drafts by ID, or all of them. |
| [prune](msg-drafts-server-prune.md) | Trash superseded and already-sent drafts. |

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

- `mail_cli msg drafts server ls`
- `mail_cli msg drafts server rm 141C4C3B 6C5A8047`
- `mail_cli --dry-run msg drafts server rm all` — Show every draft rm all would trash
- `mail_cli --dry-run msg drafts server prune --sent` — Show what a prune would trash

## Matching by subject

--by-subject also treats drafts with the same recipients and subject as copies when one of them has no Message-ID, and with --sent matches such a draft to a message sent after it. Two messages that both carry Message-IDs are the same only if the IDs agree, so it cannot merge copies that compose uploaded before 2026-09-30, when each upload got a fresh ID: remove those with 'rm' after 'ls'.

## What it reads and changes

Sent is read up to the account's fetch limit, and --sent downloads it into the cache. Under --dry-run or --read-only every move is reported and none is made.

---

[↑ mail\_cli](index.md) — [nav](nav.md)
