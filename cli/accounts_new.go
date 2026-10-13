package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"

	"golang.org/x/term"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"

	"github.com/sarielhp/clihelp"
)

// promptAccountDetails asks for whatever the chosen backend needs and returns
// the account to save. Each type has its own required fields and its own
// sensible folder names, which is the whole of what differs between them.
func promptAccountDetails(acctType, name, mode string) (cfg_acc.AccountConfig, error) {
	newAcc := cfg_acc.AccountConfig{
		Name:        name,
		Type:        acctType,
		AccountType: mode,
	}

	switch acctType {
	case "jmap":
		username, err := promptRequired("Username (JMAP Email)")
		if err != nil {
			return newAcc, err
		}
		newAcc.Username = username
		if newAcc.Password, err = promptSecret("Password (JMAP API Token)", "your-jmap-api-token"); err != nil {
			return newAcc, err
		}
		if newAcc.SessionURL, err = promptUser("JMAP Session URL", "https://api.fastmail.com/jmap/session"); err != nil {
			return newAcc, err
		}
		newAcc.SpamFolder, newAcc.ReceivedFolder, _ = cfg_acc.FolderDefaults(acctType)
		newAcc.UnspamLearn = "Inbox"
		newAcc.Whitelist = []string{}
		newAcc.Blacklist = []string{}
		newAcc.Rules = []cfg_acc.Rule{}

	case "gmail":
		username, err := promptRequired("Username (Gmail Email)")
		if err != nil {
			return newAcc, err
		}
		newAcc.Username = username
		if newAcc.Password, err = promptSecret("Password (or dummy value like 'oauth-auth')", "oauth-auth"); err != nil {
			return newAcc, err
		}
		if newAcc.IMAPHost, err = promptUser("IMAP Host", "imap.gmail.com:993"); err != nil {
			return newAcc, err
		}
		newAcc.SpamFolder, newAcc.ReceivedFolder, _ = cfg_acc.FolderDefaults(acctType)
		newAcc.UnspamLearn = "INBOX"

	case "outlook":
		username, err := promptRequired("Username (Outlook Email)")
		if err != nil {
			return newAcc, err
		}
		newAcc.Username = username
		newAcc.SpamFolder, newAcc.ReceivedFolder, _ = cfg_acc.FolderDefaults(acctType)
		newAcc.UnspamLearn = "Inbox"

	default:
		return newAcc, ErrInvalidAccountType(acctType)
	}
	return newAcc, nil
}

// promptRequired asks until it gets a non-empty answer, or gives up on EOF.
//
// An account with no username cannot authenticate, but `account new` used to
// write it anyway: the prompt has no default, so pressing Enter -- or a
// redirected stdin closing -- produced an entry that failed much later at
// `account login` with "account username is missing", which points at the
// config rather than at what actually happened.
func promptRequired(prompt string) (string, error) {
	for attempt := range 3 {
		value, err := promptUser(prompt, "")
		if err != nil {
			return "", err
		}
		if value != "" {
			return value, nil
		}
		if attempt < 2 {
			fmt.Printf("%s This cannot be empty.\n", app.PrefixWarn)
		}
	}
	return "", fmt.Errorf("%s is required", strings.ToLower(prompt))
}

// promptReader is the one buffered reader over standard input.
//
// Every prompt used to construct its own bufio.Reader over os.Stdin. A buffered
// reader fills from the underlying file, so the first prompt pulled in every
// line already available, answered with the first and discarded the rest; the
// next prompt then read EOF. At a terminal this is invisible, because lines
// arrive one at a time as they are typed. Through a pipe or a here-doc every
// answer after the first was lost -- and the old code hid it, because losing an
// answer and pressing Enter both produced the default.
//
// It is rebuilt when os.Stdin changes so that a test can substitute a pipe.
var (
	promptReaderMu  sync.Mutex
	promptReaderIn  *os.File
	promptReaderBuf *bufio.Reader
)

func promptReader() *bufio.Reader {
	promptReaderMu.Lock()
	defer promptReaderMu.Unlock()
	if promptReaderBuf == nil || promptReaderIn != os.Stdin {
		promptReaderIn = os.Stdin
		promptReaderBuf = bufio.NewReader(os.Stdin)
	}
	return promptReaderBuf
}

// The prompt functions below write to os.Stdout deliberately. They are paired
// with a read from stdin, so the prompt has to reach the terminal the user is
// typing at; sending it to a redirected writer would leave them staring at a
// blank line.
func promptUser(prompt string, defaultValue string) (string, error) {
	fmt.Printf("%s", prompt)
	if defaultValue != "" {
		fmt.Printf(" [%s]", defaultValue)
	}
	fmt.Printf(": ")

	input, err := promptReader().ReadString('\n')
	if err != nil {
		return "", noAnswer(prompt, err)
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue, nil
	}
	return input, nil
}

// noAnswer reports that there was nobody to ask.
//
// An empty line and an unreadable stdin are different answers, and these
// prompts used to treat them alike by returning the default for both. With
// stdin redirected, `account new jmap work` therefore wrote a session URL
// pointing at Fastmail and, worse, stored the literal placeholder
// "your-jmap-api-token" as the account's password -- values the user never saw,
// saved as though they had typed them. A default is what someone chooses by
// pressing Enter, not what we pick on their behalf when no one is there.
func noAnswer(prompt string, err error) error {
	return fmt.Errorf("%w: cannot read an answer for %q: %w", ErrUsage(), prompt, err)
}

func promptSecret(prompt string, defaultValue string) (string, error) {
	return promptSecretWithInput(promptReader(), int(os.Stdin.Fd()), term.IsTerminal(int(os.Stdin.Fd())), prompt, defaultValue)
}

func promptSecretWithInput(r *bufio.Reader, fd int, isTerm bool, prompt, defaultValue string) (string, error) {
	fmt.Printf("%s", prompt)
	if defaultValue != "" {
		fmt.Printf(" [%s]", defaultValue)
	}
	fmt.Printf(": ")

	if isTerm {
		passBytes, err := term.ReadPassword(fd)
		fmt.Println()
		if err != nil {
			return "", noAnswer(prompt, err)
		}
		trimmed := strings.TrimSpace(string(passBytes))
		if trimmed == "" {
			return defaultValue, nil
		}
		return trimmed, nil
	}

	line, err := r.ReadString('\n')
	if err != nil {
		return "", noAnswer(prompt, err)
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return defaultValue, nil
	}
	return trimmed, nil
}

var (
	flagAccountNewType        string
	flagAccountNewTest        bool
	flagAccountNewInteractive bool
)

func accountNewCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "add",
		Aliases:         []string{"new"},
		Title:           "account add <type> [name]",
		Description:     "Add a new account template to config.json.",
		LongDescription: "Prompts for whatever the chosen backend needs and writes the entry to config.json. With --interactive the prompts follow the same guided flow as `account setup`: provider-specific tips, the Gmail credentials.json copy, and a connectivity check before the command returns.",
		UsageLine:       "mail_cli account add <jmap|gmail|outlook> [name] [flags]",
		Parameters: []clihelp.Param{
			{Name: "<jmap|gmail|outlook>", Description: "Account backend provider type.", Complete: CompleteAccountProviders},
			{Name: "[name]", Description: "Unique name for the account."},
		},
		Options: []clihelp.Option{
			clihelp.Enum(&flagAccountNewType, "--type <type>", []string{"regular", "test"}, "regular", "Account type: regular or test"),
			clihelp.Bool(&flagAccountNewTest, "--test", false, "Mark as test account (shortcut for --type test)"),
			clihelp.Bool(&flagAccountNewInteractive, "--interactive", false, "Guide setup with provider tips and a connection check"),
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli account add gmail personal-gmail"},
			{Line: "mail_cli account add jmap work-fastmail"},
			{Line: "mail_cli account add outlook uni-outlook --test"},
			{Line: "mail_cli account add gmail personal-gmail --interactive", Description: "Guided setup with a connection check"},
		},
		Args: clihelp.RangeArgs(1, 2),
		Run: func(ctx *clihelp.Context) error {
			args := ctx.Args
			acctType := strings.ToLower(args[0])
			name := ""
			if len(args) > 1 {
				name = args[1]
			}

			_, configPath, err := cfg_g.ResolveConfigPath(session.Config)
			if err != nil {
				return err
			}
			fc, err := cfg_g.LoadConfigFile(configPath)
			if err != nil {
				return err
			}

			acctMode := flagAccountNewType
			if flagAccountNewTest {
				acctMode = "test"
			}
			if acctMode != "regular" && acctMode != "test" {
				return ErrInvalidAccountMode(acctMode)
			}

			if name == "" {
				var promptErr error
				if name, promptErr = promptUser("Account name (unique identifier)", ""); promptErr != nil {
					return promptErr
				}
				if name == "" {
					return fmt.Errorf("account name is required")
				}
			}

			// Check for duplicate account name
			if fc.Accounts == nil {
				empty := []cfg_acc.AccountConfig{}
				fc.Accounts = &empty
			}
			for _, acc := range *fc.Accounts {
				if strings.EqualFold(acc.Name, name) {
					return ErrAccountExists(name)
				}
			}

			if flagAccountNewInteractive {
				return addAccountInteractive(session, configPath, fc, acctType, name, acctMode)
			}

			newAcc, err := promptAccountDetails(acctType, name, acctMode)
			if err != nil {
				return err
			}

			*fc.Accounts = append(*fc.Accounts, newAcc)
			if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
				return err
			}

			fmt.Fprintf(session.errOut(), "%s Successfully added new %s account entry %q to config file.\n", app.PrefixSuccess, strings.ToUpper(acctType), name)
			if acctType == "outlook" || acctType == "gmail" {
				fmt.Fprintf(session.out(), "Please run: %s account login %s to perform OAuth authentication\n", os.Args[0], name)
			} else {
				fmt.Fprintf(session.out(), "Please run: %s account test %s to verify connection\n", os.Args[0], name)
			}
			return nil
		},
	}
}

// addAccountInteractive is the --interactive path of `account add`: the same
// guided prompts as `account setup`, scoped to a provider and name already given
// on the command line, followed by the same connectivity check.
//
// It is a second path rather than a flag on promptAccountDetails because the two
// differ in more than verbosity: the terse prompts ask for the same fields but
// carry no provider tips, never copy Gmail's credentials.json into place, and
// never verify that what was written can actually authenticate. Reusing the
// setup helpers keeps the two entry points from drifting apart.
func addAccountInteractive(session *Session, configPath string, fc *cfg_g.FileConfig, acctType, name, acctMode string) error {
	newAcc, err := promptSetupAccountDetails(session, acctType, name)
	if err != nil {
		return err
	}
	// The setup helpers fix AccountType at "regular"; the command line decides
	// whether this is a test account, exactly as the non-interactive path does.
	newAcc.AccountType = acctMode

	if err := saveNewSetupAccount(session, configPath, fc, newAcc); err != nil {
		return err
	}

	fmt.Fprintf(session.errOut(), "%s Successfully added new %s account entry %q to config file.\n", app.PrefixSuccess, strings.ToUpper(acctType), name)
	_ = verifySetupAccount(session, newAcc)
	return nil
}
