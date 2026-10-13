---
title: 'mail_cli config'
has_children: true
---

# mail\_cli config

Use config show, get, set, and unset for preferences. Run config check to validate configuration, or config llm test to check the configured model endpoint. Account credentials are managed under account; local maintenance lives under utils.

## Usage

```
mail_cli config [subcommand]
```

## Subcommands

| Command | Description |
|---------|-------------|
| [show](config-show.md) | Display the active configuration values. |
| [get](config-get.md) | Print a configuration parameter's value. |
| [set](config-set.md) | Set a configuration parameter value. |
| [unset (reset)](config-unset.md) | Reset a configuration parameter to default. |
| [check (validate)](config-check.md) | Validate config, DNS, and Bogofilter. |
| [llm](config-llm.md) | Test the configured LLM endpoint. |

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

- `mail_cli config show`
- `mail_cli config get mutt_alias_file`
- `mail_cli config set mutt_alias_file ~/.mutt/aliases`
- `mail_cli config set spam_folder Junk`
- `mail_cli config unset score_threshold`
- `mail_cli config check`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
