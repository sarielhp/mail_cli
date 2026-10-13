package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/contacts"

	"github.com/sarielhp/clihelp"
)

// The arguments of write and send: attachments given as patterns, recipients
// that turn out to be files, and the completion of both.

// expandAttachments turns the -a values into paths. A value holding a wildcard
// is expanded here, so `-a "*.pdf"` works quoted -- unquoted, the shell
// expands it, and every match after the first lands among the recipients
// (see checkRecipientsAreNotFiles). A pattern that matches nothing is an error:
// sending the message without the attachment the user asked for is worse.
//
// A value that names an existing file is that file, wildcard characters or
// not: a script attaching "report[1].pdf" means that file, and as a pattern
// it would match "report1.pdf" or nothing.
func expandAttachments(values []string) ([]string, error) {
	var paths []string
	seen := map[string]bool{}
	for _, raw := range values {
		value := expandHome(strings.TrimSpace(raw))
		if value == "" {
			continue
		}
		matches := []string{value}
		if _, err := os.Stat(value); err != nil && strings.ContainsAny(value, "*?[") {
			var err error
			if matches, err = filepath.Glob(value); err != nil || len(matches) == 0 {
				return nil, fmt.Errorf("%w: attachment pattern %q matches no file", app.ErrUsage, raw)
			}
		}
		for _, m := range matches {
			if !seen[m] {
				seen[m] = true
				paths = append(paths, m)
			}
		}
	}
	return paths, nil
}

// expandHome expands a leading "~/", which a quoted pattern keeps.
func expandHome(path string) string {
	rest, ok := strings.CutPrefix(path, "~/")
	if !ok {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, rest)
}

// checkRecipientsAreNotFiles refuses a recipient that is an existing file and
// cannot be an address or a contact: what `-a *.pdf` leaves behind when the
// shell expands the pattern, since -a takes only the first match. Writing to
// "b.pdf" would fail late, at sending, and say nothing about why.
func checkRecipientsAreNotFiles(book *contacts.Book, recipients []string) error {
	for _, item := range recipients {
		for _, part := range strings.Split(item, ",") {
			part = strings.TrimSpace(part)
			if part == "" || strings.Contains(part, "@") || isContactName(book, part) {
				continue
			}
			if info, err := os.Stat(part); err == nil && info.Mode().IsRegular() {
				return fmt.Errorf("%w: %q is a file, not a recipient; attach files with -a, "+
					"once per file or with a quoted pattern: -a \"*.pdf\"", app.ErrUsage, part)
			}
		}
	}
	return nil
}

// isContactName reports whether name resolves to a contact the way a recipient
// does when the message is sent: by name or nickname, ignoring case.
func isContactName(book *contacts.Book, name string) bool {
	if book == nil {
		return false
	}
	for _, c := range book.Contacts {
		if strings.EqualFold(c.Name, name) {
			return true
		}
		for _, nick := range c.Nicknames {
			if strings.EqualFold(nick, name) {
				return true
			}
		}
	}
	return false
}

// withCompleter attaches a completion callback to an option built by clihelp.
func withCompleter(opt clihelp.Option, complete func(string) []string) clihelp.Option {
	opt.Complete = complete
	return opt
}

// sessionCfg is the session's configuration, or nil without a session.
func sessionCfg(session *Session) *cfg_g.Config {
	if session == nil {
		return nil
	}
	return session.Config
}

// loadCompletionConfig reads the configuration for a completer; tests replace
// it. It never writes config.json: completion runs on every Tab.
var loadCompletionConfig = cfg_g.LoadConfigNoWrite

// completionConfig is the configuration a completer should read. Shell
// completion (__complete) is answered before BeforeRun, so the session still
// holds the empty bootstrap Config: its CacheRoot is unset, which sent the
// contacts history lookup to os.UserCacheDir instead of the configured
// download_dir, and its blacklist was empty. A session that has not been
// bootstrapped loads the configuration itself, without writing it.
func completionConfig(session *Session) *cfg_g.Config {
	config := sessionCfg(session)
	if config != nil && config.CacheRoot != "" {
		return config
	}
	loaded, err := loadCompletionConfig()
	if err != nil || loaded == nil {
		return config
	}
	if loaded.ConfigDir == "" {
		if home, hErr := os.UserHomeDir(); hErr == nil {
			loaded.ConfigDir = filepath.Join(home, ".config", app.AppName)
		}
	}
	return loaded
}

// recipientCompletionLimit bounds the candidates: a menu longer than a screen
// is not read. recipientSuggestPool is how many ranked contacts are examined to
// find them, since the best-ranked are not always the ones that start with
// what was typed.
const (
	recipientCompletionLimit = 30
	recipientSuggestPool     = 500
)

// completeRecipients completes a recipient from the contacts book and the
// addresses mail was exchanged with, ranked as `contacts suggest` ranks them.
// Only the part after the last comma is completed, so "-c alice,bo" works.
//
// A candidate is the spelling that starts with what was typed -- a nickname,
// or else an address -- because bash and zsh drop any candidate that does not.
func completeRecipients(session *Session) func(string) []string {
	return func(toComplete string) []string {
		keep, query := splitAtLastComma(toComplete)
		config := completionConfig(session)
		book, _ := contacts.LoadBook(contacts.DefaultPath(config))
		history, _ := contacts.LoadHistory(contacts.DefaultHistoryPath(config))
		var blacklist []string
		if config != nil {
			blacklist = config.Policy.Blacklist
		}
		// Candidates that start with what was typed come first, and are all a
		// shell that matches prefixes can use. Suggest also matches inside names
		// and addresses ("smith"), which only fish can show; those are offered
		// when nothing starts with the query, so they do not crowd it.
		var prefixed, inside []string
		seen := map[string]bool{}
		for _, sc := range contacts.Suggest(book, history, blacklist, query, recipientSuggestPool) {
			spelling := recipientSpelling(sc.Contact, query)
			if spelling == "" || seen[strings.ToLower(spelling)] {
				continue
			}
			seen[strings.ToLower(spelling)] = true
			cand := keep + spelling + "\t" + recipientDescription(sc.Contact)
			if strings.HasPrefix(strings.ToLower(spelling), strings.ToLower(query)) {
				prefixed = append(prefixed, cand)
			} else {
				inside = append(inside, cand)
			}
		}
		if len(prefixed) == 0 {
			prefixed = inside
		}
		if len(prefixed) > recipientCompletionLimit {
			prefixed = prefixed[:recipientCompletionLimit]
		}
		return prefixed
	}
}

// recipientSpelling picks what to insert for c: a nickname that starts with
// query, else an address that does, else the primary address -- which fish,
// matching anywhere in a word, still offers when query matched only the name.
func recipientSpelling(c contacts.Contact, query string) string {
	lower := strings.ToLower(query)
	for _, nick := range c.Nicknames {
		if strings.HasPrefix(strings.ToLower(nick), lower) {
			return nick
		}
	}
	for _, addr := range c.Emails {
		if strings.HasPrefix(strings.ToLower(addr), lower) {
			return addr
		}
	}
	return c.PrimaryEmail()
}

func recipientDescription(c contacts.Contact) string {
	if c.Name == "" {
		return c.PrimaryEmail()
	}
	return c.Name + " <" + c.PrimaryEmail() + ">"
}

// completeAttachments completes the file after the last comma of an -a value.
// Without a comma it returns nothing, and the shell's own file completion --
// which knows about directories, quoting and spacing -- answers instead.
func completeAttachments(toComplete string) []string {
	keep, rest := splitAtLastComma(toComplete)
	if keep == "" {
		return nil
	}
	dir, base := filepath.Split(rest)
	entries, err := os.ReadDir(expandHome(orDot(dir)))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, base) || (strings.HasPrefix(name, ".") && !strings.HasPrefix(base, ".")) {
			continue
		}
		if e.IsDir() {
			name += "/"
		}
		out = append(out, keep+dir+name)
	}
	return out
}

func orDot(dir string) string {
	if dir == "" {
		return "."
	}
	return dir
}

// splitAtLastComma separates what a shell must keep from the item being
// completed: "a.pdf,b" is "a.pdf," and "b".
func splitAtLastComma(s string) (keep, rest string) {
	i := strings.LastIndex(s, ",")
	if i < 0 {
		return "", s
	}
	return s[:i+1], s[i+1:]
}
