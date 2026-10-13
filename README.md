# mail_cli

[![CI](https://github.com/sarielhp/mail_cli/actions/workflows/ci.yml/badge.svg)](https://github.com/sarielhp/mail_cli/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/sarielhp/mail_cli)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A high-performance terminal and CLI email client engineered for power users. Native OAuth for Google Gmail and Microsoft 365 / Outlook, native JMAP for Fastmail, an interactive Bubbletea TUI, and Mutt-style UNIX scripting pipes — without `offlineimap`, `mbsync`, or sync-and-script hell.

---

## Why mail_cli?

For decades, `mutt` and `neomutt` have been the gold standard for terminal email. But in 2026, running them requires an intricate, brittle stack:
* Maintaining external sync daemons (`mbsync`, `offlineimap`) and handling UIDVALIDITY errors.
* Managing external SMTP dispatchers (`msmtp`).
* Managing external search indexers (`notmuch`).
* Managing custom OAuth2 token renewal scripts that break whenever Google or Microsoft change auth flows.
* Writing hundreds of lines of obscure `.muttrc` config just to view basic threads or HTML attachments.

`mail_cli` replaces that entire stack with a single, standalone binary.

### Comparison Matrix

| Feature | `mail_cli` | `NeoMutt` | `aerc` | `Himalaya` | `Alpine` |
| :--- | :---: | :---: | :---: | :---: | :---: |
| **Primary Interface** | **TUI + Rich CLI** | TUI | TUI | CLI (basic TUI) | TUI |
| **Native Gmail OAuth** | **Yes (Direct)** | No (Scripted) | Partial | Partial | No |
| **Native Outlook Graph** | **Yes (Device Code)**| No (Scripted) | No | No | No |
| **Native Fastmail JMAP** | **Yes (Direct)** | No | No | No | No |
| **External Sync Needed** | **None** | `mbsync`/`offlineimap` | Optional | Optional | None |
| **Setup Time** | **< 3 minutes** | 1–3 hours | 15–30 mins | 15 mins | 20 mins |
| **Local Search** | **Fast built-in** | Needs `notmuch` | Server / grep | Server | Built-in |
| **Recipient Auto-Complete**| **Yes (History+Rank)**| Aliases only | Basic | Basic | Addressbook |
| **Spam / Political Unsub**| **Built-in** | External scripts | No | No | No |
| **Local LLM Summaries** | **Yes (Ollama/OpenAI)**| No | External script | No | No |
| **Bidi / RTL Support** | **Built-in** | Patch required | Limited | Terminal-dep | No |

---

## Features

* **Direct API Integration**: Native REST API client for Gmail with OAuth2, Microsoft Graph API for Outlook / Office 365 with Device Code auth, and JMAP API for Fastmail.
* **Modern Interactive TUI**: Dual-pane split view, expandable folder/label tree, thread collapsing, inline attachment inspection, and full Hebrew/Bidi RTL text rendering.
* **Mutt-Style CLI Scripting**:
  * Outbound transmission: `mail_cli msg send -s "Subject" user@example.com < message.txt`
  * Direct replies: `mail_cli msg reply -g <message-id>` (interactive in `$EDITOR` or headless via `--headless`)
  * Pre-formatted RFC822 pipeline: `mail_cli msg send --raw < patch.eml` (drop-in compatibility with `git send-email`)
* **Ranked Recipient Auto-Complete**:
  * Frequency and recency ranking prioritizing contacts you actively communicate with.
  * In-TUI `<Tab>` completion and contact aliases.
  * Mutt alias file import (`mail_cli contacts import-mutt ~/.muttrc`) and Google Contacts sync.
* **Intelligent Mail Hygiene**:
  * Local Bogofilter bayesian spam training (`mail_cli spam sweep`).
  * Automated political fundraising email audit and one-click unsubscription.
* **Optional AI Capabilities**:
  * One-key message summarization and key deadline extraction via local LLMs (Ollama) or cloud APIs.

---

## Installation

Releases are source-only — no prebuilt binaries are published — so install with
`go install` or build from the tagged source. Either needs Go 1.26 or newer.

### Go Install
```bash
go install github.com/sarielhp/mail_cli@latest
```

This puts `mail_cli` in `$(go env GOPATH)/bin`.

### Build from Source
```bash
git clone https://github.com/sarielhp/mail_cli.git
cd mail_cli
make
```

---

## Quick Start

### 1. Interactive Onboarding (`mail_cli account setup`)
Run the interactive setup wizard to configure your first email account in under 3 minutes:
```bash
mail_cli account setup
```
The wizard guides you through selecting your provider:
1. **Fastmail / JMAP**: Fast setup using an API token ([docs/setup/fastmail.md](docs/setup/fastmail.md)).
2. **Google Gmail**: Connect through your own Google Cloud OAuth client, a free one-time setup of about
   3 minutes; mail_cli ships no shared Gmail credentials ([docs/setup/gmail.md](docs/setup/gmail.md)).
3. **Microsoft 365 / Outlook**: Connect via Azure Device Code login ([docs/setup/outlook.md](docs/setup/outlook.md)).

### 2. Launch the Interactive TUI
```bash
mail_cli tui
```
* Press <kbd>?</kbd> in any view to view the keybinding cheat sheet.
* Navigate with <kbd>j</kbd>/<kbd>k</kbd>, switch folders with <kbd>Tab</kbd>, and toggle split preview with <kbd>p</kbd>.
* Reply with <kbd>r</kbd> (or <kbd>g</kbd> for group reply), compose with <kbd>c</kbd>, archive with <kbd>a</kbd>.
* Long URLs stay in the message text with an abbreviated display ending in `...`.
  On terminals supporting hyperlinks, clicking opens the complete URL. Other
  terminals display the complete URL.

---

## Command Line Cheat Sheet

The CLI has 14 top level commands. Browsing and composing stay direct; other actions
are grouped by the thing you want to manage. Use `<command> -h` for a short
list, `<command> -H` for details, and `-E <command>` for examples.

| Commands | Purpose |
|---|---|
| `scan`, `search`, `write`, `tui` | Find, classify, browse mail, and write new messages |
| `msg` | Show, send, reply, drafts, recent mail, export, archive, block |
| `label`, `mbox` | Manage server folders and local mail archives |
| `spam`, `rule` | Train spam filters and manage routing rules |
| `calendar`, `contacts` | Manage events and recipient information |
| `account` | Guided setup, accounts, login, migration |
| `config` | Preferences, validation, LLM checks |
| `utils` | Cache, terminal colors, diagnostics, completion, manual pages |

Earlier root spellings such as `send`, `show`, `drafts`, `last`, `setup`,
`cache`, and `completion` still work as hidden compatibility commands. Their
flags and behavior are preserved; help and completion teach the grouped paths.
The previous `compose`, `msg compose`, and `tools` spellings also remain
available as hidden shortcuts for `write` and `utils`.
`tui write` is equivalent to `write`, including recipients, flags, and draft
resumption, for wrappers that already invoke `mail_cli tui`.

```bash
# Browse & Read
mail_cli %                          # Scan the inbox (% alone denotes INBOX)
mail_cli %work:receipts             # Scan a specific folder on a specific account
mail_cli msg show <message-id>          # Display email body and attachments
mail_cli search "invoice 2026"      # Fast indexed search across downloaded messages
mail_cli msg last 20                # Recent mail across all folders

# Compose & Send
mail_cli write                    # Compose interactively with $EDITOR and TUI review
mail_cli msg send -s "Update" boss@co.com < notes.txt    # Send from stdin
mail_cli msg send -s "Report" -a report.pdf user@co.com  # Send with attachment
mail_cli msg send --raw < patch.eml                      # Send pre-formatted RFC822 (git send-email)

# Reply
mail_cli msg reply 18fb1234abcd         # Open interactive reply in $EDITOR with quoted text
mail_cli msg reply -g 18fb1234abcd      # Group reply (reply-all) to all recipients
mail_cli msg reply --headless --body "Ack" 18fb1234abcd  # Headless instant reply

# Contacts & Hygiene
mail_cli contacts suggest "ali"     # Search auto-complete recipient candidates
mail_cli contacts import-mutt ~/.muttrc  # Import existing Mutt aliases
mail_cli spam sweep                 # Move spam out of inbox and train filter

# Setup & Maintenance
mail_cli account setup              # Guided account setup
mail_cli config llm test            # Check the configured LLM endpoint
mail_cli utils cache prune 30       # Prune old local cached messages
mail_cli utils completion install   # Install shell completion
```

---

## Documentation

* [Fastmail Setup Guide](docs/setup/fastmail.md)
* [Gmail Setup Guide](docs/setup/gmail.md)
* [Outlook Setup Guide](docs/setup/outlook.md)
* [Architecture & Layering Principles](docs/ARCHITECTURE.md)
* [CLI Help Reference](docs/clihelp/)

---

## License

This project is licensed under the [MIT License](LICENSE).
