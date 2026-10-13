---
title: 'mail_cli utils doctor'
has_children: true
parent: 'mail_cli utils'
---

# mail\_cli utils doctor

Diagnose the setup: verify credentials, connectivity and mail flow. It proves the account is writable by creating a label and a message and then deleting both, so it is refused under --read-only and --dry-run.

## Usage

```
mail_cli utils doctor run
```

## Subcommands

| Command | Description |
|---------|-------------|
| [run](utils-doctor-run.md) | Execute connection and integration tests. |

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

- `mail_cli utils doctor run`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
