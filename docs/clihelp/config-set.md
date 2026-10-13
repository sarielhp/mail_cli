---
title: 'mail_cli config set'
parent: 'mail_cli config'
---

# mail\_cli config set

Set a configuration parameter to a new value. Keys that belong to an account are written to the account -A selects.

## Usage

```
mail_cli config set <key> <value>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<key>` | The configuration parameter name. |
| `<value>` | The value to set for the parameter. |

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

- `mail_cli config set spam_folder Junk`
- `mail_cli config set limit 50`
- `mail_cli -A work config set badge 💼` — Tag the work account in All Inboxes
- `mail_cli config set mutt_alias_file ~/.mutt/aliases`
- `mail_cli config set llm_endpoint http://localhost:4000/v1/chat/completions`
- `mail_cli config set llm_api_key sk-litellm-master-key`
- `mail_cli config set llm_auto_summary true`
- `mail_cli config set attachment_save_dir ~/info/misc/%YY/%MM/%DD/`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
