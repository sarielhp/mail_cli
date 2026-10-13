#!/usr/bin/env bash
set -euo pipefail

# Ensure we are in the repository root
cd "$(dirname "$0")/.."

TEMPLATE_PATH="examples/config.json.template"

cat << 'EOF' > "$TEMPLATE_PATH"
{
  "username": "your-email@example.com",
  "password": "your-password-or-app-password",
  "imap_host": "imap.example.com:993",
  "download_dir": "~/.cache/mail_cli",
  "limit": 1000,
  "score_threshold": 0,
  "spam_folder": "Spam",
  "received_folder": "Inbox",
  "spam_learn": "LearnSpam",
  "unspam_learn": "LearnUnSpam",
  "allowed_languages": [
    "english"
  ],
  "block_political": true,
  "auto_unsubscribe": false,
  "accounts": [
    {
      "name": "personal",
      "type": "gmail",
      "username": "your-email@gmail.com",
      "password": "your-gmail-app-password",
      "imap_host": "imap.gmail.com:993",
      "spam_folder": "[Gmail]/Spam",
      "received_folder": "received",
      "spam_learn": "LearnSpam",
      "unspam_learn": "LearnUnSpam",
      "badge": "📮",
      "aliases": [
        "gmail"
      ],
      "whitelist": [],
      "blacklist": [],
      "rules": []
    }
  ],
  "browser": "browser-name",
  "llm": {
    "endpoint": "http://localhost:4000/v1/chat/completions",
    "api_key": "sk-litellm-master-key",
    "model": "llama3.1",
    "temperature": 0.2,
    "max_tokens": 350,
    "reasoning_effort": "none"
  }
}
EOF

echo "Template created at $TEMPLATE_PATH"
