package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/contacts"
	"github.com/sarielhp/mail_cli/mailops"
	"github.com/sarielhp/mail_cli/report"
)

// ContactService implements cli.ContactOps to manage local contacts and aliases.
type ContactService struct {
	Rep report.Reporter
	Out io.Writer
}

func (s ContactService) out() io.Writer {
	if s.Out != nil {
		return s.Out
	}
	return os.Stdout
}

func (s ContactService) rep() report.Reporter {
	return report.Or(s.Rep)
}

// loadContactBook reads the contact book at config's default path.
func loadContactBook(config *cfg_g.Config) (*contacts.Book, error) {
	return contacts.LoadBook(contacts.DefaultPath(config))
}

// printContacts loads the contact book and prints the table produced by
// selectRows, which picks the rows to show: Filter for a plain listing, Search
// for a field query.
func (s ContactService) printContacts(config *cfg_g.Config, selectRows func(*contacts.Book, string) []contacts.Contact, arg string) error {
	book, err := loadContactBook(config)
	if err != nil {
		return err
	}
	return contacts.FormatTable(s.out(), selectRows(book, arg))
}

// List displays stored contacts matching an optional filter.
func (s ContactService) List(config *cfg_g.Config, filter string) error {
	return s.printContacts(config, contacts.Filter, filter)
}

// Search queries stored contacts across all fields.
func (s ContactService) Search(config *cfg_g.Config, query string) error {
	return s.printContacts(config, contacts.Search, query)
}

// ImportMutt parses a Mutt alias file and updates mutt contacts in the book.
func (s ContactService) ImportMutt(config *cfg_g.Config, customPath string) error {
	targetPath := customPath
	if targetPath == "" && config != nil {
		targetPath = config.Paths.MuttAliases
	}
	if targetPath == "" {
		return fmt.Errorf("no mutt alias file specified: provide a path or run 'mail_cli config set paths.mutt_aliases <path>'")
	}

	newContacts, err := contacts.ParseMuttFile(targetPath)
	if err != nil {
		return err
	}

	bookPath := contacts.DefaultPath(config)
	book, err := contacts.LoadBook(bookPath)
	if err != nil {
		return err
	}

	contacts.UpdateSource(book, "mutt", newContacts)
	if err := contacts.SaveBook(bookPath, book); err != nil {
		return err
	}

	s.rep().Successf("Imported %d contacts from %s into %s\n", len(newContacts), targetPath, bookPath)
	return nil
}

// SyncGoogle fetches contacts from Google People API for a specific or active account.
func (s ContactService) SyncGoogle(config *cfg_g.Config, accountName string) error {
	targetAcc := accountName
	if targetAcc == "" && config != nil {
		targetAcc = string(config.SelectedAccount)
	}

	acc := findAccount(config, targetAcc)
	if acc == nil {
		return fmt.Errorf("account %q not found in configuration", targetAcc)
	}
	if acc.Type != "gmail" {
		return fmt.Errorf("account %q is type %q; google contacts sync requires a gmail account", acc.Name, acc.Type)
	}

	httpClient, err := mailops.NewGoogleOAuthClient(s.rep(), cfg_g.WithAccount(config, *acc))
	if err != nil {
		return fmt.Errorf("failed to authorize Google client for %q: %w", acc.Name, err)
	}

	newContacts, err := contacts.FetchGoogleContacts(context.Background(), httpClient, acc.Name)
	if err != nil {
		return err
	}

	sourcePrefix := fmt.Sprintf("google:%s", acc.Name)
	bookPath := contacts.DefaultPath(config)
	book, err := contacts.LoadBook(bookPath)
	if err != nil {
		return err
	}

	contacts.UpdateSource(book, sourcePrefix, newContacts)
	if err := contacts.SaveBook(bookPath, book); err != nil {
		return err
	}

	s.rep().Successf("Synced %d Google contacts for account %q into %s\n", len(newContacts), acc.Name, bookPath)
	return nil
}

// Path prints the location of the local contacts file.
func (s ContactService) Path(config *cfg_g.Config) error {
	path := contacts.DefaultPath(config)
	fmt.Fprintln(s.out(), path)
	return nil
}

// Suggest queries ranked recipient suggestions from curated contacts and cache history.
func (s ContactService) Suggest(config *cfg_g.Config, query string) error {
	bookPath := contacts.DefaultPath(config)
	book, _ := contacts.LoadBook(bookPath)

	historyPath := contacts.DefaultHistoryPath(config)
	history, _ := contacts.LoadHistory(historyPath)

	var blacklist []string
	if config != nil {
		blacklist = config.Policy.Blacklist
	}

	results := contacts.Suggest(book, history, blacklist, query, 15)
	return contacts.FormatSuggestTable(s.out(), results)
}

// Harvest scans cached messages across all accounts to build the suggested recipient cache.
func (s ContactService) Harvest(config *cfg_g.Config) error {
	if config == nil {
		return fmt.Errorf("configuration is required for harvest")
	}

	targetMap := make(map[string]*contacts.ScoredContact)
	totalMsgs := 0
	blacklist := config.Policy.Blacklist

	if len(config.Accounts) > 0 {
		for _, acc := range config.Accounts {
			accDir := config.AccountCacheFor(acc)
			count, err := contacts.HarvestAccountCache(accDir, acc.Name, blacklist, targetMap)
			if err == nil {
				totalMsgs += count
			}
		}
	}

	history := &contacts.HistoryBook{
		Updated: time.Now().UTC(),
		Entries: make([]contacts.ScoredContact, 0, len(targetMap)),
	}
	for _, entry := range targetMap {
		history.Entries = append(history.Entries, *entry)
	}

	historyPath := contacts.DefaultHistoryPath(config)
	if err := contacts.SaveHistory(historyPath, history); err != nil {
		return err
	}

	s.rep().Successf("Harvested %d messages; indexed %d suggested recipients into %s\n", totalMsgs, len(history.Entries), historyPath)
	return nil
}

// Purge removes an email address from the suggestion history.
func (s ContactService) Purge(config *cfg_g.Config, email string) error {
	historyPath := contacts.DefaultHistoryPath(config)
	return contacts.PurgeAddress(historyPath, email)
}

func findAccount(config *cfg_g.Config, name string) *cfg_acc.AccountConfig {
	if config == nil {
		return nil
	}
	for i := range config.Accounts {
		if strings.EqualFold(config.Accounts[i].Name, name) {
			return &config.Accounts[i]
		}
	}
	if len(config.Accounts) > 0 && name == "" {
		return &config.Accounts[0]
	}
	return nil
}
