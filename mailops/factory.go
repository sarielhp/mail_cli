package mailops

import (
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"log/slog"
	"net/http"
	"strings"

	"github.com/sarielhp/mail_cli/backend/gmail"
	"github.com/sarielhp/mail_cli/backend/jmap"
	"github.com/sarielhp/mail_cli/backend/mbox"
	"github.com/sarielhp/mail_cli/backend/outlook"
	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cache/virtual"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"
)

// BuildAccountLocalConfig creates a per-account configuration struct.
func BuildAccountLocalConfig(acc cfg_acc.AccountConfig, config *cfg_g.Config) cfg_g.Config {
	localCfg := *config
	if acc.Username != "" {
		localCfg.Username = acc.Username
	}
	if acc.Password != "" {
		localCfg.Password = acc.Password
	}
	if acc.IMAPHost != "" {
		localCfg.IMAPHost = acc.IMAPHost
	}
	if acc.SpamFolder != "" {
		localCfg.SpamFolder = acc.SpamFolder
	}
	if acc.ReceivedFolder != "" {
		localCfg.ReceivedFolder = acc.ReceivedFolder
	}
	// SpamLearn and UnspamLearn are per-account settings in the config file, and
	// were the only such overrides this function did not carry. So scan/scan.go
	// moved blacklisted mail to the *global* folder while cfg_g/helpers.go filed
	// the matching rule against the *account's* — two paths disagreeing about
	// where the same message belongs, in a program that may hold several accounts
	// with different folder names.
	if acc.SpamLearn != "" {
		localCfg.SpamLearn = acc.SpamLearn
	}
	if acc.UnspamLearn != "" {
		localCfg.UnspamLearn = acc.UnspamLearn
	}
	localCfg.SelectedAccount = cfg_g.NameOf(acc)
	localCfg.AccountType = acc.AccountType
	if localCfg.AccountType == "" {
		localCfg.AccountType = "regular"
	}
	localCfg.Rules = []cfg_acc.Rule{}
	if acc.Rules != nil {
		localCfg.Rules = acc.Rules
	}
	localCfg.Whitelist = []string{}
	if acc.Whitelist != nil {
		localCfg.Whitelist = acc.Whitelist
	}
	localCfg.Blacklist = []string{}
	if acc.Blacklist != nil {
		localCfg.Blacklist = acc.Blacklist
	}
	// The account's cache directory, beside the root it lives under. These
	// used to be one field that this function rewrote from the root to the
	// account directory, so code holding the session's config read the root
	// where it meant the account.
	localCfg.AccountCache = config.AccountCacheFor(acc)
	if acc.ReadOnly {
		localCfg.ReadOnly = true
	}
	return localCfg
}

// InstantiateBackendClient instantiates the underlying MailClient for the given
// account type. rep is where that backend reports progress; a nil rep discards.
func InstantiateBackendClient(acc cfg_acc.AccountConfig, localCfg *cfg_g.Config, rep report.Reporter) (mailclient.MailClient, error) {
	switch strings.ToLower(acc.Type) {
	case "gmail":
		return gmail.NewClient(acc, localCfg, rep), nil
	case "jmap":
		return jmap.NewClient(acc, localCfg, rep), nil
	case "outlook":
		return outlook.NewClient(acc, localCfg, rep), nil
	default:
		return nil, fmt.Errorf("unsupported account type: %q", acc.Type)
	}
}

// IsAccountSelected checks if the account matches the currently active account.
func IsAccountSelected(acc cfg_acc.AccountConfig, config *cfg_g.Config) bool {
	selected, err := cfg_g.Lookup(config.Accounts, string(config.SelectedAccount))
	if err != nil {
		return false
	}
	return strings.EqualFold(selected.Name, acc.Name)
}

// migrateAccountCacheLayout is H4's cache-layout migration for what lives
// inside one account's (already-migrated) cache directory: the folder
// indexes named by the account's own cached label listing, and every
// self-describing virtual mailbox. Both are best-effort -- a missing or
// unreadable label cache just means there is nothing yet to migrate, and
// this runs on every client construction, so a transient failure here is
// not worth failing the whole command over; the next run tries again.
func migrateAccountCacheLayout(downloadDir cachedir.Account) {
	if downloadDir.IsZero() {
		return
	}

	store := &cache.DiskCacheStore{Dir: downloadDir}
	if items, err := store.GetLabelItems(); err == nil && len(items) > 0 {
		names := make([]string, 0, len(items))
		for _, item := range items {
			name := item.FullName
			if name == "" {
				name = item.Name
			}
			if name != "" {
				names = append(names, name)
			}
		}
		if _, err := folderindex.MigrateSanitizedNames(downloadDir, names); err != nil {
			slog.Debug("folder index sanitization migration failed",
				slog.String("dir", downloadDir.Path()), slog.Any("error", err))
		}
	}

	if _, err := virtual.MigrateSanitizedNames(downloadDir); err != nil {
		slog.Debug("virtual mailbox sanitization migration failed",
			slog.String("dir", downloadDir.Path()), slog.Any("error", err))
	}
}

// NewMailClient constructs a fully initialized and wrapped MailClient for the
// account, reporting its progress through rep.
func NewMailClient(acc cfg_acc.AccountConfig, config *cfg_g.Config, rep report.Reporter) (mailclient.MailClient, error) {
	// The account's own cache directory has to be migrated before
	// BuildAccountLocalConfig computes localCfg.AccountCache from it (H4):
	// that computation joins config.CacheRoot with the *current*
	// sanitization of acc.Name, which is exactly the path this call moves
	// the pre-H4 directory to if it is still sitting under the old one.
	cfg_g.MigrateDirSanitization(config.CacheRoot, acc.Name)
	localCfg := BuildAccountLocalConfig(acc, config)
	migrateAccountCacheLayout(localCfg.AccountCache)

	client, err := InstantiateBackendClient(acc, &localCfg, rep)
	if err != nil {
		return nil, err
	}

	if localCfg.ReadOnly || acc.ReadOnly {
		client = mailclient.NewReadOnlyMailClient(client, acc.Name)
	}

	// Every account caches into its own directory -- BuildAccountLocalConfig
	// gives each one config.CacheRoot/<account> -- so there is nothing for one
	// to collide with in another, and dirlock guards concurrent writers. A
	// non-selected account used to get a no-op store, which CheckingMailClient
	// then read as "this client is inactive" and used to short-circuit every
	// fetch to an empty result.
	cs := &cache.DiskCacheStore{Dir: localCfg.AccountCache}

	return mailclient.NewCheckingMailClient(client, acc.Name, cs), nil
}

// The three places a command has to name a specific backend. They live here,
// behind the factory, so nothing above has to import gmail or outlook.

// TokenPath is where an account's OAuth token lives, or "" for a backend that
// has none.
func TokenPath(configDir string, acc cfg_acc.AccountConfig) (string, error) {
	switch strings.ToLower(acc.Type) {
	case "gmail":
		return gmail.GetTokenPath(&cfg_g.Config{
			SelectedAccount: cfg_g.NameOf(acc),
			Paths:           cfg_g.Paths{ConfigDir: configDir},
		}), nil
	case "outlook":
		return outlook.GetTokenPath(&cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: configDir}}, acc.Name)
	default:
		return "", nil
	}
}

// RunBackendTests exercises the Gmail REST surface. It is a Gmail-specific
// diagnostic, which is why it is named here rather than on the interface.
func RunBackendTests(rep report.Reporter, config *cfg_g.Config) error {
	return gmail.RunRESTTests(rep, config)
}

// NewGoogleOAuthClient is the OAuth client provider calendar/google is wired
// with. Gmail and Google Calendar share one token and one scope list.
func NewGoogleOAuthClient(rep report.Reporter, config *cfg_g.Config) (*http.Client, error) {
	return gmail.NewOAuthHTTPClient(rep, config)
}

// OpenMbox builds the read-only, caching client stack over a local mbox file.
func OpenMbox(config *cfg_g.Config, mboxPath string, cacheDir cachedir.Account) (mailclient.MailClient, error) {
	if len(config.Accounts) == 0 {
		return nil, fmt.Errorf("mbox view requires a synthetic account in the config")
	}
	acc := config.Accounts[0]

	raw := mbox.NewClient(acc, config, mboxPath)
	readOnly := mailclient.NewReadOnlyMailClient(raw, acc.Name)
	return NewCheckingClient(readOnly, acc.Name, &cache.DiskCacheStore{Dir: cacheDir}), nil
}

// NewCheckingClient wraps a client in the label cache.
func NewCheckingClient(delegate mailclient.MailClient, accName string, store cache.Store) mailclient.MailClient {
	return mailclient.NewCheckingMailClient(delegate, accName, store)
}
