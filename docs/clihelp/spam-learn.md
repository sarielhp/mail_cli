---
title: 'mail_cli spam learn'
parent: 'mail_cli spam'
---

# mail\_cli spam learn

Train Bogofilter on all messages currently in the Spam folder, then move them to the LearnSpam folder.

## Usage

```
mail_cli spam learn [dest_label]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[dest_label]` | Destination label (default: SpamLearn folder) |

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
| `--ham <label>` | Train on known-good mail, not Spam. |
| `--force` | Bypass the trained-message ledger and re-train everything. |

## Examples

- `mail_cli spam learn` — Train the filter on everything in the Spam folder
- `mail_cli spam learn --ham %received` — Train on a folder of known-good mail
- `mail_cli spam unmark abc123de` — This one was not spam: train as ham and move it back
- `mail_cli spam allow add friend@example.com` — Never treat this sender as spam
- `mail_cli spam block add spammer@example.com` — Always treat this sender as spam
- `mail_cli spam learn %Archive/Spam`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
