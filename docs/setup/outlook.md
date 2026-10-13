# Microsoft 365 / Outlook Account Setup

`mail_cli` supports Microsoft 365, Outlook.com, Live.com, and enterprise/university Exchange Online accounts via Microsoft Graph and OAuth2 Device Code authentication.

Setup takes under **2 minutes**.

---

## 1. Configure the Account

Run the setup wizard:

```bash
mail_cli account setup
```

1. Select option **`3`** (Microsoft 365 / Outlook).
2. Enter a friendly account name (e.g., `work-outlook` or `uni-email`).
3. Enter your Microsoft email address (e.g., `user@outlook.com` or `employee@company.com`).

Alternatively, add the account directly:

```bash
mail_cli account add outlook work-outlook
```

---

## 2. Authenticate via Device Code

When prompted by `mail_cli account setup` (or via `mail_cli account login work-outlook`):

1. `mail_cli` displays an authentication banner:
   ```
   ==================================================================
                 OUTLOOK OAUTH DEVICE AUTHENTICATION
   ==================================================================
   To sign in, please open a web browser on the page:

       https://microsoft.com/devicelogin

   And enter the following code: ABC1DEF2 to authenticate.
   ==================================================================
   ```
2. Open `https://microsoft.com/devicelogin` in any web browser (on your computer, phone, or tablet).
3. Enter the 8-character code displayed in your terminal.
4. Sign in with your Microsoft 365 credentials and approve the application permissions.
5. Your terminal will automatically detect completion, save your authentication token to `~/.config/mail_cli/token_<name>.json`, and verify server connectivity.

---

## 3. Verify Connection

```bash
mail_cli account test work-outlook
mail_cli label list
mail_cli %
```
