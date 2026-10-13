---
title: 'mail_cli rule remote'
has_children: true
parent: 'mail_cli rule'
---

# mail\_cli rule remote

Manage remote Gmail server-side filters and forwarding rules.

Remote filters are server-side mail rules enforced directly on incoming messages by the mail provider (e.g. Gmail) before mail_cli ever connects. In mail_cli, filters and rules share the same conceptual model:

  1. Define rules locally in config.json using 'rule add' or 'rule add-title'.
  2. Export rules to the server as remote filters using 'rule push' (or 'rule export'). This translates local rules into server filters (auto-labeling, archiving, or forwarding) and marks them as exported.
  3. If a remote filter already exists with a different action, overwrite it with 'rule push --force'.
  4. Inspect active server-side filters with 'rule remote list'.
  5. Delete an active server filter and its matching local rule with 'rule remote rm'.

## Usage

```
mail_cli rule remote <subcommand>
```

## Subcommands

| Command | Description |
|---------|-------------|
| [list](rule-remote-list.md) | List server-side filters for this account. |
| [rm](rule-remote-rm.md) | Delete a server filter and its local rule. |

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

- `mail_cli rule remote list`
- `mail_cli rule remote rm 1`

## Filters are Server-Side Rules

Gmail filters run on Google servers 24/7 as incoming mail arrives. mail_cli manages them declaratively: you create routing rules locally with 'rule add', then push them to Gmail with 'rule push'.

## Creating Filters via Export / Push

Run 'mail_cli rule push' to export all unexported local rules to Gmail server filters. If a remote filter conflicts with a local rule, pass '--force' (or 'rule push force') to overwrite the remote filter.

---

[↑ mail\_cli](index.md) — [nav](nav.md)
