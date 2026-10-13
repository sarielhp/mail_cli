# Fastmail / JMAP Account Setup

Fastmail is a first-class citizen in `mail_cli` via the modern JMAP (JSON Meta Application Protocol, RFC 8620 / RFC 8621) API standard.

Setup takes under **1 minute** using an API token.

---

## 1. Generate a Fastmail API Token

1. Log into your Fastmail account in your web browser.
2. Navigate to **Settings** (gear icon) -> **Password & Security**.
3. Under **API Tokens**, click **New API Token**.
4. Configure the token:
   - **Name**: `mail_cli`
   - **Access**: Select **Full access** (or choose custom access ensuring **Mail** is granted read/write permissions).
5. Click **Generate Token**.
6. Copy the generated token string (starts with `fmu1-...`).

---

## 2. Configure `mail_cli`

Run the guided onboarding wizard:

```bash
mail_cli account setup
```

1. Select option **`1`** (Fastmail / JMAP).
2. Enter a friendly account name (e.g., `fastmail` or `work`).
3. Enter your Fastmail email address (e.g., `user@fastmail.com`).
4. Paste the API Token you generated above.
5. Accept the default JMAP Session URL (`https://api.fastmail.com/jmap/session`).
6. Confirm connection verification.

Alternatively, you can add the account directly via:

```bash
mail_cli account add jmap fastmail
```

---

## 3. Verify Connection

Test that `mail_cli` can reach Fastmail and list your folders:

```bash
mail_cli account test fastmail
mail_cli label list
```

You are now ready to use `mail_cli`:

```bash
mail_cli tui         # Open interactive terminal mail browser
mail_cli %           # Scan your Inbox
mail_cli msg send        # Send an email
```
