package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"

	"github.com/sarielhp/clihelp"
)

func runAccountList(session *Session) error {
	_, configPath, err := cfg_g.ResolveConfigPath(session.Config)
	if err != nil {
		return err
	}
	fc, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		return err
	}

	if wantJSON() {
		return session.emitJSON(accountListing(session, *fc.Accounts))
	}

	if len(*fc.Accounts) == 0 {
		fmt.Fprintln(session.out(), "No accounts configured.")
		return nil
	}
	fmt.Fprintf(session.out(), "Configured accounts (%d):\n", len(*fc.Accounts))
	for i, acc := range *fc.Accounts {
		selected := ""
		if session.Config.IsSelected(acc) {
			selected = " *"
		}
		accType := acc.AccountType
		if accType == "" {
			accType = "regular"
		}
		displayName := acc.GetDisplayName()
		if displayName != acc.Name {
			fmt.Fprintf(session.out(), "%d. %s [%s] (%s, type: %s)%s\n", i+1, displayName, acc.Name, acc.Type, accType, selected)
		} else {
			fmt.Fprintf(session.out(), "%d. %s (%s, type: %s)%s\n", i+1, acc.Name, acc.Type, accType, selected)
		}
	}
	return nil
}

func runAccountAssociate(session *Session, accountName, progName string) error {
	_, configPath, err := cfg_g.ResolveConfigPath(session.Config)
	if err != nil {
		return err
	}
	fc, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		return err
	}

	for i := range *fc.Accounts {
		if strings.EqualFold((*fc.Accounts)[i].Name, accountName) {
			acc := &(*fc.Accounts)[i]
			if acc.Aliases == nil {
				acc.Aliases = []string{}
			}
			acc.Aliases = append(acc.Aliases, progName)
			if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
				return err
			}
			fmt.Fprintf(session.errOut(), "%s Successfully associated program name %q with account %q.\n", app.PrefixSuccess, progName, accountName)
			return nil
		}
	}
	return ErrAccountNotFound(accountName)
}

func runAccountTest(session *Session, args []string) error {
	if len(args) > 0 {
		name := args[0]
		matched := ""
		for _, a := range session.Config.Accounts {
			if strings.EqualFold(a.Name, name) || strings.EqualFold(a.GetDisplayName(), name) {
				matched = string(cfg_g.NameOf(a))
				break
			}
		}
		if matched == "" {
			return ErrAccountNotFoundAlt(name)
		}
		session.Config.SelectedAccount = cfg_g.AccountName(matched)
	}
	// The client is resolved and validated for its side effect: reaching the
	// server is what "verified" means here.
	if _, err := GetValidatedClient(session); err != nil {
		return err
	}
	fmt.Fprintf(session.errOut(), "%s Access successfully verified for account %q!\n", app.PrefixSuccess, session.Config.SelectedAccount)
	return nil
}

func runAccountCalendar(session *Session, args []string) error {
	_, configPath, err := cfg_g.ResolveConfigPath(session.Config)
	if err != nil {
		return err
	}
	fc, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		calName := ""
		for _, acc := range *fc.Accounts {
			if acc.CalendarManager {
				calName = acc.Name
				break
			}
		}
		if calName == "" {
			fmt.Fprintln(session.out(), "No calendar manager account designated.")
			return nil
		}
		fmt.Fprintf(session.out(), "Calendar manager account: %s\n", calName)
		return nil
	}

	accountName := args[0]
	for i := range *fc.Accounts {
		if strings.EqualFold((*fc.Accounts)[i].Name, accountName) {
			(*fc.Accounts)[i].CalendarManager = true
			if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
				return err
			}
			fmt.Fprintf(session.errOut(), "%s Successfully designated account %q as calendar manager.\n", app.PrefixSuccess, accountName)
			return nil
		}
	}
	return ErrAccountNotFound(accountName)
}

func cleanupAccountToken(session *Session, configPath string, targetAcc *cfg_acc.AccountConfig) {
	tokenPath, err := session.accounts().TokenPath(filepath.Dir(configPath), *targetAcc)
	if err != nil || tokenPath == "" {
		return
	}

	if err := os.Remove(tokenPath); err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(session.errOut(), "%s Failed to delete old token file %s: %v\n", app.PrefixWarn, tokenPath, err)
		}
		return
	}
	fmt.Fprintf(session.errOut(), "%s Deleted old token file: %s\n", app.PrefixInfo, tokenPath)
}

func runAccountLogin(session *Session, args []string) error {
	_, configPath, err := cfg_g.ResolveConfigPath(session.Config)
	if err != nil {
		return err
	}
	fc, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		return err
	}

	name := string(session.Config.SelectedAccount)
	if len(args) > 0 {
		name = args[0]
	}
	var targetAcc *cfg_acc.AccountConfig
	for i := range *fc.Accounts {
		if strings.EqualFold((*fc.Accounts)[i].Name, name) {
			targetAcc = &(*fc.Accounts)[i]
			break
		}
	}
	if targetAcc == nil {
		return ErrAccountNotFoundAlt(name)
	}

	// Check the account's own settings before the old token is deleted. The
	// post-login check rejects an account with, say, no username before it ever
	// contacts the server, so this used to delete a working token, never sign
	// in, and then report "signed in ... the credential has been saved".
	if invalid := cfg_g.ValidateAccountParams(*targetAcc); invalid != nil {
		return accountSettingsError(targetAcc.Name, invalid)
	}

	session.Config.SelectedAccount = cfg_g.NameOf(*targetAcc)
	cleanupAccountToken(session, configPath, targetAcc)

	client, err := session.clients().ForSelected(session.Config)
	if err != nil {
		return err
	}
	if err := validateAfterLogin(session.out(), client); err != nil {
		return err
	}
	fmt.Fprintf(session.errOut(), "%s Successfully authenticated and logged in account %q!\n", app.PrefixSuccess, targetAcc.Name)
	return nil
}

// accountSettingsError explains an account whose configuration is incomplete,
// and for the field a user most often lacks -- the username, which older
// versions of `account new` let through empty -- says how to set it.
func accountSettingsError(name string, err error) error {
	hint := fmt.Sprintf("fix the account's settings in its account.json under the config directory, then run `%s account login %s` again", app.AppName, name)
	if strings.Contains(err.Error(), "username") {
		hint = fmt.Sprintf("set its email address with `%s -A %s config set username you@example.com`, then run `%s account login %s` again",
			app.AppName, name, app.AppName, name)
	}
	return fmt.Errorf("account %q cannot sign in: %w; %s", name, err, hint)
}

// validateAfterLogin checks the credential just obtained, retrying once.
//
// The check runs immediately after a device-code exchange, and a server can be
// slow to honour a token it has only just issued -- Microsoft Graph answered
// the first request after login with a timeout. Reporting that as a failure is
// wrong twice over: the credential was obtained and written to disk, so the
// login did work, and the user is left believing they must do the whole
// browser dance again.
// loginRetryDelay gives the server a moment before the retry. A variable so a
// test does not have to wait it out.
var loginRetryDelay = 3 * time.Second

func validateAfterLogin(w io.Writer, client mailclient.Reader) error {
	err := client.Validate()
	if err == nil {
		return nil
	}

	fmt.Fprintf(w, "%s First check after sign-in did not answer (%v); retrying...\n", app.PrefixWarn, err)
	time.Sleep(loginRetryDelay)

	if retryErr := client.Validate(); retryErr != nil {
		return fmt.Errorf("signed in, but the account could not be reached: %w. "+
			"The credential has been saved, so try `%s account test` before signing in again",
			retryErr, app.AppName)
	}
	return nil
}

func runAccountRename(session *Session, oldName, newName string) error {
	if strings.TrimSpace(newName) == "" {
		return fmt.Errorf("new account name cannot be empty")
	}

	_, configPath, err := cfg_g.ResolveConfigPath(session.Config)
	if err != nil {
		return err
	}
	fc, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		return err
	}

	oldAccIdx := -1
	for i, acc := range *fc.Accounts {
		if strings.EqualFold(acc.Name, oldName) || (acc.DisplayName != "" && strings.EqualFold(acc.DisplayName, oldName)) {
			oldAccIdx = i
			break
		}
	}

	if oldAccIdx == -1 {
		return ErrAccountNotFound(oldName)
	}

	for i, acc := range *fc.Accounts {
		if i == oldAccIdx {
			continue
		}
		if strings.EqualFold(acc.Name, newName) || (acc.DisplayName != "" && strings.EqualFold(acc.DisplayName, newName)) {
			return ErrAccountExists(newName)
		}
	}

	(*fc.Accounts)[oldAccIdx].DisplayName = newName
	if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
		return err
	}

	fmt.Fprintf(session.errOut(), "%s Successfully renamed account %q to %q (display name updated).\n", app.PrefixSuccess, oldName, newName)
	return nil
}

// updateAccountConfig applies mutate to the account named by accountName and
// saves the config, returning the account's canonical name.
func updateAccountConfig(session *Session, accountName string, mutate func(*cfg_acc.AccountConfig)) (string, error) {
	_, configPath, err := cfg_g.ResolveConfigPath(session.Config)
	if err != nil {
		return "", err
	}
	fc, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		return "", err
	}

	accIdx := -1
	for i, acc := range *fc.Accounts {
		if strings.EqualFold(acc.Name, accountName) || (acc.DisplayName != "" && strings.EqualFold(acc.DisplayName, accountName)) {
			accIdx = i
			break
		}
	}
	if accIdx == -1 {
		return "", ErrAccountNotFound(accountName)
	}

	mutate(&(*fc.Accounts)[accIdx])
	if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
		return "", err
	}
	return (*fc.Accounts)[accIdx].Name, nil
}

// runAccountUserName sets the personal name used in outgoing mail, leaving the
// account's display name (its label) untouched.
func runAccountUserName(session *Session, accountName, userName string) error {
	if strings.TrimSpace(userName) == "" {
		return fmt.Errorf("user name cannot be empty")
	}
	name, err := updateAccountConfig(session, accountName, func(acc *cfg_acc.AccountConfig) {
		acc.UserName = userName
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(session.errOut(), "%s Set sender name for account %q to %q.\n", app.PrefixSuccess, name, userName)
	return nil
}

// runAccountUserEmail sets the address outgoing mail is sent from, which may
// differ from the account's login username (a Fastmail alias, for example).
func runAccountUserEmail(session *Session, accountName, userEmail string) error {
	userEmail = strings.TrimSpace(userEmail)
	if !strings.Contains(userEmail, "@") {
		return fmt.Errorf("user email %q is not a valid address", userEmail)
	}
	name, err := updateAccountConfig(session, accountName, func(acc *cfg_acc.AccountConfig) {
		acc.UserEmail = userEmail
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(session.errOut(), "%s Set send address for account %q to %q.\n", app.PrefixSuccess, name, userEmail)
	return nil
}

func runAccountDelete(session *Session, name string) error {
	_, configPath, err := cfg_g.ResolveConfigPath(session.Config)
	if err != nil {
		return err
	}
	fc, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		return err
	}

	foundIdx := -1
	for i, acc := range *fc.Accounts {
		if strings.EqualFold(acc.Name, name) || (acc.DisplayName != "" && strings.EqualFold(acc.DisplayName, name)) {
			foundIdx = i
			break
		}
	}

	if foundIdx == -1 {
		return ErrAccountNotFound(name)
	}

	targetAcc := (*fc.Accounts)[foundIdx]
	*fc.Accounts = append((*fc.Accounts)[:foundIdx], (*fc.Accounts)[foundIdx+1:]...)

	if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
		return err
	}

	cleanupAccountToken(session, configPath, &targetAcc)

	fmt.Fprintf(session.errOut(), "%s Successfully deleted account %q and cleaned up associated token files.\n", app.PrefixSuccess, targetAcc.Name)
	return nil
}

func newAccountListCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "list",
		Description: "List configured accounts.",
		UsageLine:   "mail_cli account list",
		Args:        clihelp.NoArgs,
		Run: func(_ *clihelp.Context) error {
			return runAccountList(session)
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli account list"},
		},
	}
}

func newAccountAssociateCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "link",
		Aliases:         []string{"associate"},
		Title:           "account link <name> <program>",
		Description:     "Link program name for auto-selection.",
		LongDescription: "Associate a program/symlink name with an account for automatic account selection.",
		UsageLine:       "mail_cli account link <account_name> <program_name>",
		Parameters: []clihelp.Param{
			{Name: "<account_name>", Description: "The existing account name to associate with.", Complete: CompleteAccountNames},
			{Name: "<program_name>", Description: "Program name that selects this account."},
		},
		Args: clihelp.ExactArgs(2),
		Run: func(ctx *clihelp.Context) error {
			return runAccountAssociate(session, ctx.Args[0], ctx.Args[1])
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli account link work-account work_mail"},
		},
	}
}

func newAccountTestCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "test",
		Title:       "account test [account_name]",
		Description: "Check an account's config and connection.",
		UsageLine:   "mail_cli account test [account_name]",
		Parameters: []clihelp.Param{
			{Name: "[account_name]", Description: "Account name to test (defaults to currently selected account).", Complete: CompleteAccountNames},
		},
		Args: clihelp.MaximumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return runAccountTest(session, ctx.Args)
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli account test"},
			{Line: "mail_cli account test work-fastmail"},
		},
	}
}

func newAccountCalendarCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "calendar",
		Aliases:     []string{"cal"},
		Title:       "account calendar [account_name]",
		Description: "Show or set the calendar manager account.",
		UsageLine:   "mail_cli account calendar [account_name]",
		Parameters: []clihelp.Param{
			{Name: "[account_name]", Description: "Account name to configure calendar for.", Complete: CompleteAccountNames},
		},
		Args: clihelp.MaximumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return runAccountCalendar(session, ctx.Args)
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli account calendar"},
			{Line: "mail_cli account calendar personal-gmail"},
		},
	}
}

func newAccountLoginCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "login",
		Title:           "account login [account_name]",
		Description:     "OAuth login for Gmail or Outlook.",
		LongDescription: "Perform interactive OAuth login for a Gmail or Outlook account.",
		UsageLine:       "mail_cli account login [account_name]",
		Parameters: []clihelp.Param{
			{Name: "[account_name]", Description: "Account name to re-authenticate (defaults to active account).", Complete: CompleteAccountNames},
		},
		Args: clihelp.MaximumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return runAccountLogin(session, ctx.Args)
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli account login"},
			{Line: "mail_cli account login personal-gmail"},
		},
	}
}

func newAccountRenameCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "rename",
		Title:       "account rename <old_name> <new_name>",
		Description: "Rename an existing account's display name.",
		UsageLine:   "mail_cli account rename <old_name> <new_name>",
		Parameters: []clihelp.Param{
			{Name: "<old_name>", Description: "The current name of the account to rename.", Complete: CompleteAccountNames},
			{Name: "<new_name>", Description: "The new name for the account."},
		},
		Args: clihelp.ExactArgs(2),
		Run: func(ctx *clihelp.Context) error {
			return runAccountRename(session, ctx.Args[0], ctx.Args[1])
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli account rename old-gmail new-gmail"},
		},
	}
}

// accountUserFieldSpec describes an `account user-<field>` subcommand, which
// sets one field of the identity used for outgoing mail from an account.
type accountUserFieldSpec struct {
	name        string
	title       string
	description string
	longDesc    string
	valueName   string
	valueDesc   string
	accountDesc string
	example     string
	set         func(*Session, string, string) error
}

func newAccountUserFieldCmd(session *Session, spec accountUserFieldSpec) clihelp.Command {
	return clihelp.Command{
		Name:            spec.name,
		Title:           spec.title,
		Description:     spec.description,
		LongDescription: spec.longDesc,
		UsageLine:       "mail_cli account " + spec.name + " <account_name> " + spec.valueName,
		Parameters: []clihelp.Param{
			{Name: "<account_name>", Description: spec.accountDesc, Complete: CompleteAccountNames},
			{Name: spec.valueName, Description: spec.valueDesc},
		},
		Args: clihelp.ExactArgs(2),
		Run: func(ctx *clihelp.Context) error {
			return spec.set(session, ctx.Args[0], ctx.Args[1])
		},
		Examples: []clihelp.Example{
			{Line: spec.example},
		},
	}
}

func newAccountUserNameCmd(session *Session) clihelp.Command {
	return newAccountUserFieldCmd(session, accountUserFieldSpec{
		name:        "user-name",
		title:       "account user-name <account> <name>",
		description: "Set the name shown in outgoing mail.",
		longDesc:    "Set the personal name placed in the From header of mail you send from an account. It is separate from the account's display name, which only labels the account in listings and the TUI; the display name is never used as the sender.",
		valueName:   "<user_name>",
		valueDesc:   "Name to show in the From header, e.g. \"Sariel Har-Peled\".",
		accountDesc: "The account whose outgoing name to set.",
		example:     "mail_cli account user-name fastmail \"Sariel Har-Peled\"",
		set:         runAccountUserName,
	})
}

func newAccountUserEmailCmd(session *Session) clihelp.Command {
	return newAccountUserFieldCmd(session, accountUserFieldSpec{
		name:        "user-email",
		title:       "account user-email <account> <email>",
		description: "Set the address outgoing mail is sent from.",
		longDesc:    "Set the From address used when sending from an account, which may differ from the login username -- a Fastmail alias or a verified send-as identity, for example. The login credential is unchanged; only the From header and the submission envelope use this address.",
		valueName:   "<email>",
		valueDesc:   "Address to send from, e.g. an alias of the account.",
		accountDesc: "The account whose send address to set.",
		example:     "mail_cli account user-email fastmail sariel@sarielhp.org",
		set:         runAccountUserEmail,
	})
}

func newAccountDeleteCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "rm",
		NoAbbrev:    true,
		Aliases:     []string{"delete", "del"},
		Title:       "account rm <account_name>",
		Description: "Delete an account and its credentials.",
		UsageLine:   "mail_cli account rm <account_name>",
		Parameters: []clihelp.Param{
			{Name: "<account_name>", Description: "The name of the account to delete.", Complete: CompleteAccountNames},
		},
		Args: clihelp.ExactArgs(1),
		Options: []clihelp.Option{
			clihelp.Bool(&flagAccountDeleteYes, "-y, --yes", false, "Proceed without the confirmation prompt."),
		},
		Run: func(ctx *clihelp.Context) error {
			if err := session.Confirm(fmt.Sprintf("delete account %q and its credentials", ctx.Args[0]),
				flagAccountDeleteYes); err != nil {
				return declined(session, err)
			}
			return runAccountDelete(session, ctx.Args[0])
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli account rm temp-account"},
		},
	}
}

// AccountsCmd returns the clihelp.Command for the account command.
func AccountsCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "account",
		Group:       "Setup",
		Description: "Manage and list configured mail accounts.",
		UsageLine:   "mail_cli account <subcommand> [args...]",
		Subcommands: []clihelp.Command{
			newAccountListCmd(session),
			accountNewCmd(session),
			newAccountAssociateCmd(session),
			newAccountTestCmd(session),
			newAccountCalendarCmd(session),
			newAccountLoginCmd(session),
			newAccountRenameCmd(session),
			newAccountUserNameCmd(session),
			newAccountUserEmailCmd(session),
			newAccountDeleteCmd(session),
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli account list"},
			{Line: "mail_cli account new personal-gmail"},
			{Line: "mail_cli account test"},
			{Line: "mail_cli account rename old-gmail new-gmail"},
			{Line: "mail_cli account rm temp-account"},
		},
	}
}

// accountJSON is one account as a script sees it.
type accountJSON struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
	AccountType string `json:"account_type"`
	Selected    bool   `json:"selected"`
}

// accountListing renders the accounts for --json.
func accountListing(session *Session, accounts []cfg_acc.AccountConfig) []accountJSON {
	out := make([]accountJSON, 0, len(accounts))
	for _, acc := range accounts {
		accType := acc.AccountType
		if accType == "" {
			accType = "regular"
		}
		out = append(out, accountJSON{
			Name:        acc.Name,
			DisplayName: acc.GetDisplayName(),
			Type:        acc.Type,
			AccountType: accType,
			Selected:    session.Config.IsSelected(acc),
		})
	}
	return out
}
