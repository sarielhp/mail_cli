package cfg_g

import (
	"fmt"
	"strings"

	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cfg_acc"
)

// Account identifiers, and what happens when one does not name exactly one
// account.
//
// This was hand-rolled at several sites under rules that disagreed, and the
// disagreement was not harmless: an identifier that matched nothing resolved to
// whichever account happened to be first in the file, so a typo in -A ran the
// command against the wrong account -- and nine packages reach an account and
// then write the config back.

// ErrNoAccounts reports a config with no accounts at all.
var ErrNoAccounts = fmt.Errorf("no accounts configured")

// ErrNoSenderAccount reports that multiple accounts exist but none was specified or configured for sending.
var ErrNoSenderAccount = fmt.Errorf("no sender account specified; use -A, -1, -2, or configure default_send_account")

// AmbiguousAccountError reports an identifier that names more than one account.
type AmbiguousAccountError struct {
	Ident      string
	Candidates []string
}

func (e *AmbiguousAccountError) Error() string {
	return fmt.Sprintf("account %q is ambiguous: it matches %s (name them individually)",
		e.Ident, strings.Join(e.Candidates, ", "))
}

// UnknownAccountError reports an identifier that names no account, and says
// what is configured so the caller can see the typo.
type UnknownAccountError struct {
	Ident     string
	Available []string
}

func (e *UnknownAccountError) Error() string {
	if len(e.Available) == 0 {
		return fmt.Sprintf("account %q not found: no accounts are configured", e.Ident)
	}
	return fmt.Sprintf("account %q not found; configured accounts: %s",
		e.Ident, strings.Join(e.Available, ", "))
}

// AccountNames lists the canonical names, for an error message or a listing.
func AccountNames(accounts []cfg_acc.AccountConfig) []string {
	names := make([]string, 0, len(accounts))
	for _, acc := range accounts {
		names = append(names, acc.Name)
	}
	return names
}

// Lookup finds the one account an identifier names.
//
// Identifiers are tried in order of how specifically they identify an account:
// the canonical name, then the display name, then an alias, and only then the
// backend type. The ordering matters because a type names every account using
// that backend -- `-A outlook` with two Outlook accounts used to resolve to
// whichever came first, silently. It is now reported as ambiguous.
//
// An exact match at a more specific level wins outright, so an account named
// "work" is still reachable as "work" even if another account's display name
// happens to be "Work".
func Lookup(accounts []cfg_acc.AccountConfig, ident string) (*cfg_acc.AccountConfig, error) {
	if len(accounts) == 0 {
		return nil, ErrNoAccounts
	}
	if ident == "" {
		return &accounts[0], nil
	}

	for _, match := range []func(cfg_acc.AccountConfig) bool{
		func(a cfg_acc.AccountConfig) bool { return strings.EqualFold(a.Name, ident) },
		func(a cfg_acc.AccountConfig) bool {
			return a.DisplayName != "" && strings.EqualFold(a.DisplayName, ident)
		},
		func(a cfg_acc.AccountConfig) bool { return matchesAlias(a, ident) },
		func(a cfg_acc.AccountConfig) bool {
			return a.Type != "" && strings.EqualFold(a.Type, ident)
		},
	} {
		found, err := single(accounts, ident, match)
		if found != nil || err != nil {
			return found, err
		}
	}

	return nil, &UnknownAccountError{Ident: ident, Available: AccountNames(accounts)}
}

// single returns the one account satisfying match, nil if none do, or an
// ambiguity error if several do.
func single(accounts []cfg_acc.AccountConfig, ident string, match func(cfg_acc.AccountConfig) bool) (*cfg_acc.AccountConfig, error) {
	var hits []int
	for i := range accounts {
		if match(accounts[i]) {
			hits = append(hits, i)
		}
	}
	switch len(hits) {
	case 0:
		return nil, nil
	case 1:
		return &accounts[hits[0]], nil
	default:
		candidates := make([]string, 0, len(hits))
		for _, i := range hits {
			candidates = append(candidates, accounts[i].Name)
		}
		return nil, &AmbiguousAccountError{Ident: ident, Candidates: candidates}
	}
}

func matchesAlias(acc cfg_acc.AccountConfig, ident string) bool {
	for _, alias := range acc.Aliases {
		if strings.EqualFold(alias, ident) {
			return true
		}
	}
	return false
}

// AccountName is an account's canonical name: the `name` field of an entry in
// accounts[], never a display name, alias or backend type that Lookup would
// also accept.
//
// Config.SelectedAccount has this type so that a raw identifier cannot be
// assigned to it. -A used to store whatever was typed, and every comparison
// against acc.Name then failed for `-A "Work Mail"` -- per-account settings
// such as save_sent_copy silently fell back to the global value. A string
// variable no longer converts implicitly: get one from NameOf, SelectAccount
// or WithAccount.
type AccountName string

// NameOf is the canonical name of an account entry.
func NameOf(acc cfg_acc.AccountConfig) AccountName { return AccountName(acc.Name) }

// WithAccount returns a copy of config that selects acc, leaving the
// caller's config alone.
//
// Selecting an account used to mean writing to the shared Config and writing
// the old value back afterwards. There was no defer, so an early return left
// the wrong account selected, and no lock, so it becomes a race the moment
// anything runs in parallel. A copy has neither problem.
func WithAccount(config *Config, acc cfg_acc.AccountConfig) *Config {
	scoped := *config
	scoped.SelectedAccount = NameOf(acc)
	return &scoped
}

// SelectAccount is WithAccount for an identifier the user typed: it resolves
// ident with Lookup, so the copy holds the canonical name, and reports an
// unknown or ambiguous identifier.
func SelectAccount(config *Config, ident string) (*Config, error) {
	acc, err := Lookup(config.Accounts, ident)
	if err != nil {
		return nil, err
	}
	return WithAccount(config, *acc), nil
}

// IsSelected reports whether acc is the selected account. No selection means
// the first account, the rule Lookup applies to an empty identifier.
func (c *Config) IsSelected(acc cfg_acc.AccountConfig) bool {
	if c == nil {
		return false
	}
	if c.SelectedAccount == "" {
		return len(c.Accounts) > 0 && strings.EqualFold(c.Accounts[0].Name, acc.Name)
	}
	return strings.EqualFold(string(c.SelectedAccount), acc.Name)
}

// AccountCacheFor is acc's cache directory under this config's cache root,
// joined the one way the program does (cachedir.ForAccount). It is zero when
// no root is configured, rather than a path relative to the working
// directory.
func (c *Config) AccountCacheFor(acc cfg_acc.AccountConfig) cachedir.Account {
	if c == nil || c.CacheRoot == "" {
		return cachedir.Account{}
	}
	return cachedir.ForAccount(c.CacheRoot, acc.Name)
}

// SelectedAccountCache is the selected account's cache directory: the one
// the client factory recorded, or, on a config it did not build -- the
// session's own -- the selected account's directory under the root.
func (c *Config) SelectedAccountCache() (cachedir.Account, error) {
	if c == nil {
		return cachedir.Account{}, ErrNoAccounts
	}
	if !c.AccountCache.IsZero() {
		return c.AccountCache, nil
	}
	acc, err := Lookup(c.Accounts, string(c.SelectedAccount))
	if err != nil {
		return cachedir.Account{}, err
	}
	return c.AccountCacheFor(*acc), nil
}
