---
title: 'mail_cli utils completion'
has_children: true
parent: 'mail_cli utils'
---

# mail\_cli utils completion

Generate or install shell tab-completion scripts

## Usage

```
mail_cli utils completion <subcommand>
```

## Subcommands

| Command | Description |
|---------|-------------|
| [bash](utils-completion-bash.md) | Generate Bash tab-completion script |
| [zsh](utils-completion-zsh.md) | Generate Zsh tab-completion script |
| [fish](utils-completion-fish.md) | Generate Fish tab-completion script |
| [keys](utils-completion-keys.md) | Print shell key bindings (Alt-H explains the command line) |
| [install](utils-completion-install.md) | Set up tab completion, the Alt-H key binding and the man page |
| [uninstall](utils-completion-uninstall.md) | Remove the installed tab completion and key binding |
| [wrap](utils-completion-wrap.md) | Generate a wrapper script with preset arguments |

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

- `mail_cli utils completion zsh` — Generate Zsh tab-completion script
- `mail_cli utils completion install` — Install tab-completions for the active shell

## Shell Tip

Tip: <Tab> to complete, Ctrl-D to list choices. Run 'completion keys' and source the result from your shell's rc file to bind Alt-H, which expands the command line and shows the help for the command it names.

---

[↑ mail\_cli](index.md) — [nav](nav.md)
