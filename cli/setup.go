package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"

	"github.com/sarielhp/clihelp"
)

// SetupCmd returns the clihelp.Command for the interactive first-run setup wizard.
func SetupCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "setup",
		Group:       "Setup",
		Description: "Configure an email account interactively.",
		UsageLine:   "mail_cli account setup",
		Examples: []clihelp.Example{
			{Line: "mail_cli account setup", Description: "Launch guided account setup wizard"},
		},
		Args: clihelp.NoArgs,
		Run: func(_ *clihelp.Context) error {
			return runSetup(session)
		},
	}
}

func runSetup(session *Session) error {
	fmt.Println("============================================================")
	fmt.Println("              mail_cli Account Setup Wizard                 ")
	fmt.Println("============================================================")
	fmt.Println("mail_cli connects directly to your email provider without")
	fmt.Println("requiring external background sync daemons.")
	fmt.Println()

	provider, err := promptProviderChoice()
	if err != nil {
		return err
	}

	defaultName := provider
	name, err := promptUser("Account name (unique identifier)", defaultName)
	if err != nil {
		return err
	}
	if name == "" {
		name = defaultName
	}

	_, configPath, err := cfg_g.ResolveConfigPath(session.Config)
	if err != nil {
		return err
	}
	fc, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		return err
	}

	// Placeholders are about to be replaced, so their names are free.
	if fc.Accounts != nil && !cfg_g.OnlyPlaceholders(*fc.Accounts) {
		for _, existing := range *fc.Accounts {
			if strings.EqualFold(existing.Name, name) {
				return ErrAccountExists(name)
			}
		}
	}

	newAcc, err := promptSetupAccountDetails(session, provider, name)
	if err != nil {
		return err
	}

	if err := saveNewSetupAccount(session, configPath, fc, newAcc); err != nil {
		return err
	}

	fmt.Println()
	fmt.Fprintf(session.errOut(), "%s Account %q (%s) successfully saved!\n", app.PrefixSuccess, name, strings.ToUpper(provider))

	_ = verifySetupAccount(session, newAcc)

	fmt.Println()
	fmt.Println("============================================================")
	fmt.Printf("Setup complete! Account %q is ready.\n", name)
	fmt.Println("Commands to try:")
	fmt.Println("  mail_cli tui         - Open interactive terminal email browser")
	fmt.Println("  mail_cli %           - Scan your inbox")
	fmt.Println("  mail_cli msg send        - Send an email from the CLI")
	fmt.Println("  mail_cli write     - Interactively compose an email")
	fmt.Println("============================================================")

	return nil
}

func promptProviderChoice() (string, error) {
	fmt.Println("Select your email provider:")
	fmt.Println("  1) Fastmail / JMAP         (Recommended, fastest setup with API token)")
	fmt.Println("  2) Gmail                   (Google Cloud OAuth2 Desktop Application)")
	fmt.Println("  3) Microsoft 365 / Outlook  (Azure Device Code flow)")
	fmt.Println()

	ans, err := promptUser("Choose provider [1/2/3]", "1")
	if err != nil {
		return "", err
	}
	switch strings.ToLower(strings.TrimSpace(ans)) {
	case "1", "jmap", "fastmail":
		return "jmap", nil
	case "2", "gmail", "google":
		return "gmail", nil
	case "3", "outlook", "m365", "office365", "microsoft":
		return "outlook", nil
	default:
		return "", fmt.Errorf("invalid provider choice: %q (expected 1, 2, or 3)", ans)
	}
}

func promptSetupAccountDetails(session *Session, provider, name string) (cfg_acc.AccountConfig, error) {
	switch provider {
	case "jmap":
		return setupJMAPAccount(name)
	case "gmail":
		configDir := ""
		if session != nil && session.Config != nil {
			configDir = session.Config.ConfigDir
		}
		return setupGmailAccount(name, configDir)
	case "outlook":
		return setupOutlookAccount(name)
	default:
		return cfg_acc.AccountConfig{}, ErrInvalidAccountType(provider)
	}
}

func setupJMAPAccount(name string) (cfg_acc.AccountConfig, error) {
	acc := cfg_acc.AccountConfig{
		Name:           name,
		Type:           "jmap",
		AccountType:    "regular",
		SpamFolder:     "Spam",
		UnspamLearn:    "Inbox",
		ReceivedFolder: "Inbox",
		Whitelist:      []string{},
		Blacklist:      []string{},
		Rules:          []cfg_acc.Rule{},
	}

	username, err := promptRequired("Email address (Username)")
	if err != nil {
		return acc, err
	}
	acc.Username = username

	fmt.Printf("Tip: Create an API token at Fastmail Settings -> Password & Security -> API Tokens\n")
	token, err := promptSecret("Fastmail API Token", "")
	if err != nil {
		return acc, err
	}
	acc.Password = token

	sessionURL, err := promptUser("JMAP Session URL", "https://api.fastmail.com/jmap/session")
	if err != nil {
		return acc, err
	}
	acc.SessionURL = sessionURL

	return acc, nil
}

func setupGmailAccount(name string, configDir string) (cfg_acc.AccountConfig, error) {
	acc := cfg_acc.AccountConfig{
		Name:           name,
		Type:           "gmail",
		AccountType:    "regular",
		Password:       "oauth-auth",
		IMAPHost:       "imap.gmail.com:993",
		SpamFolder:     "[Gmail]/Spam",
		UnspamLearn:    "INBOX",
		ReceivedFolder: "received",
		Whitelist:      []string{},
		Blacklist:      []string{},
		Rules:          []cfg_acc.Rule{},
	}

	username, err := promptRequired("Gmail address (e.g. user@gmail.com)")
	if err != nil {
		return acc, err
	}
	acc.Username = username

	if configDir != "" {
		credPath := filepath.Join(configDir, "credentials.json")
		if _, err := os.Stat(credPath); os.IsNotExist(err) {
			fmt.Println()
			fmt.Println("Gmail OAuth requires credentials.json from Google Cloud Console.")
			fmt.Println("See docs/setup/gmail.md for step-by-step instructions.")
			path, errPrompt := promptUser("Path to downloaded credentials.json (Enter to skip)", "")
			if errPrompt == nil && strings.TrimSpace(path) != "" {
				if copyErr := copyCredentialsFile(strings.TrimSpace(path), credPath); copyErr != nil {
					fmt.Printf("%s Could not copy credentials.json: %v\n", app.PrefixWarn, copyErr)
				} else {
					fmt.Printf("%s Saved credentials.json to %s\n", app.PrefixSuccess, credPath)
				}
			}
		}
	}

	return acc, nil
}

func setupOutlookAccount(name string) (cfg_acc.AccountConfig, error) {
	acc := cfg_acc.AccountConfig{
		Name:           name,
		Type:           "outlook",
		AccountType:    "regular",
		SpamFolder:     "Junk Email",
		UnspamLearn:    "Inbox",
		ReceivedFolder: "Archive",
		Whitelist:      []string{},
		Blacklist:      []string{},
		Rules:          []cfg_acc.Rule{},
	}

	username, err := promptRequired("Outlook / Microsoft 365 address (e.g. user@outlook.com)")
	if err != nil {
		return acc, err
	}
	acc.Username = username
	return acc, nil
}

func saveNewSetupAccount(session *Session, configPath string, fc *cfg_g.FileConfig, acc cfg_acc.AccountConfig) error {
	if fc.Accounts == nil {
		empty := []cfg_acc.AccountConfig{}
		fc.Accounts = &empty
	}
	// A first run finds only the placeholder the loader seeded. Appending kept
	// it first, so later runs selected it instead of the account just set up;
	// it is replaced, along with a default sender that named it.
	if cfg_g.OnlyPlaceholders(*fc.Accounts) {
		*fc.Accounts = (*fc.Accounts)[:0]
		fc.DefaultSendAccount = ""
	}
	*fc.Accounts = append(*fc.Accounts, acc)

	// A test account exists to be mutated by the live tests; it never becomes
	// the account mail is sent from by default.
	if acc.AccountType != "test" && (fc.DefaultSendAccount == "" || len(*fc.Accounts) == 1) {
		fc.DefaultSendAccount = acc.Name
	}

	if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
		return fmt.Errorf("saving config file %s: %w", configPath, err)
	}

	if session != nil && session.Config != nil {
		session.Config.Accounts = *fc.Accounts
		session.Config.DefaultSendAccount = fc.DefaultSendAccount
		if !hasAccountNamed(session.Config.Accounts, string(session.Config.SelectedAccount)) || len(session.Config.Accounts) == 1 {
			session.Config.SelectedAccount = cfg_g.NameOf(acc)
		}
	}

	return nil
}

func hasAccountNamed(accounts []cfg_acc.AccountConfig, name string) bool {
	if name == "" {
		return false
	}
	for _, a := range accounts {
		if strings.EqualFold(a.Name, name) {
			return true
		}
	}
	return false
}

func verifySetupAccount(session *Session, acc cfg_acc.AccountConfig) error {
	if acc.Type == "jmap" {
		if promptYesNo("Verify connection to server now?", true) {
			fmt.Printf("Connecting to %s...\n", acc.SessionURL)
			// Build the client for the account just added, not whichever
			// account was already selected: with more than one account
			// already configured, session.Config.SelectedAccount can still
			// be the old one (saveNewSetupAccount only reassigns it when the
			// config had none or exactly one account before this), and
			// verifying that account instead would report "Successfully
			// connected" without the new credentials ever being exercised
			// (M4).
			client, err := session.clients().ForSelected(cfg_g.WithAccount(session.Config, acc))
			if err != nil {
				fmt.Fprintf(session.errOut(), "%s Connection check failed: %v\n", app.PrefixWarn, err)
				return nil
			}
			if err := client.Validate(); err != nil {
				fmt.Fprintf(session.errOut(), "%s Validation failed: %v\n", app.PrefixWarn, err)
				return nil
			}
			fmt.Fprintf(session.errOut(), "%s Successfully connected to JMAP server!\n", app.PrefixSuccess)
		}
		return nil
	}

	if acc.Type == "gmail" || acc.Type == "outlook" {
		if promptYesNo(fmt.Sprintf("Perform %s authentication login now?", strings.ToUpper(acc.Type)), true) {
			if err := runAccountLogin(session, []string{acc.Name}); err != nil {
				fmt.Fprintf(session.errOut(), "%s Authentication deferred: %v\n", app.PrefixWarn, err)
				fmt.Fprintf(session.out(), "You can sign in later with: mail_cli account login %s\n", acc.Name)
			}
		}
	}
	return nil
}

func copyCredentialsFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0600)
}

func promptYesNo(prompt string, defaultYes bool) bool {
	defStr := "Y/n"
	if !defaultYes {
		defStr = "y/N"
	}
	ans, err := promptUser(fmt.Sprintf("%s [%s]", prompt, defStr), "")
	if err != nil {
		return defaultYes
	}
	ans = strings.ToLower(strings.TrimSpace(ans))
	if ans == "" {
		return defaultYes
	}
	return ans == "y" || ans == "yes"
}
