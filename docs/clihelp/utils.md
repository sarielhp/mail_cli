---
title: 'mail_cli utils'
has_children: true
---

# mail\_cli utils

Use utils cache for local download maintenance, utils color to inspect terminal colors, and utils completion or utils manpage for shell integration. utils doctor verifies server writes by creating and deleting a temporary folder and message; it changes the real account and refuses --read-only and --dry-run.

## Usage

```
mail_cli utils <subcommand>
```

## Subcommands

| Command | Description |
|---------|-------------|
| [cache](utils-cache.md) | Manage the local email download cache. |
| [color](utils-color.md) | Test the terminal's true-color support. |
| [doctor](utils-doctor.md) | Diagnose credentials, connectivity, mail flow. |
| [completion](utils-completion.md) | Generate or install shell tab-completion scripts |
| [manpage](utils-manpage.md) | Print the manual page, or install it for man(1) |

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

- `mail_cli utils cache prune 30` — Prune old local cached messages
- `mail_cli utils completion install` — Install shell completion
- `mail_cli utils manpage` — Print the manual page

---

[↑ mail\_cli](index.md) — [nav](nav.md)
