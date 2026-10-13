---
title: 'mail_cli label list'
parent: 'mail_cli label'
---

# mail\_cli label list

List all folders and labels in a hierarchical tree view. Pass --full to show full paths, --ext to show unread and total message counts, --nozero to omit empty folders, or --real to query the server in real-time.

## Usage

```
mail_cli label list [flags]
```

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
| `-a, --all` | List all labels, including those with zero messages |
| `-z, --nozero` | Only list folders that have at least one message in them |
| `-f, --full` | Show full label names (without hierarchical view) |
| `-e, --ext` | Show (unread/total) message counts |
| `--real` | Fetch real-time counts from server (bypasses cache) |

## Examples

- `mail_cli label list`
- `mail_cli label list --ext`
- `mail_cli label list --full --ext`
- `mail_cli label list --nozero`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
