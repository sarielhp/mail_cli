package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"

	"github.com/sarielhp/clihelp"
)

// CompleteAccountNames provides dynamic tab-completion for configured account names.
func CompleteAccountNames(toComplete string) []string {
	var matches []string
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return matches
	}
	configPath := filepath.Join(homeDir, ".config", app.AppName, "config.json")
	fc, _ := cfg_g.LoadConfigFile(configPath)
	if fc == nil || fc.Accounts == nil {
		return matches
	}
	for _, acc := range *fc.Accounts {
		if strings.HasPrefix(strings.ToLower(acc.Name), strings.ToLower(toComplete)) {
			desc := acc.Type
			if desc == "" {
				desc = "account"
			}
			matches = append(matches, acc.Name+"\t"+desc)
		}
	}
	return matches
}

// CompleteAccountProviders provides dynamic tab-completion for account types.
func CompleteAccountProviders(toComplete string) []string {
	providers := []struct {
		name string
		desc string
	}{
		{"gmail", "Google Mail"},
		{"jmap", "FastMail / Stalwart"},
		{"outlook", "Microsoft 365 / Outlook"},
	}
	var matches []string
	for _, p := range providers {
		if strings.HasPrefix(strings.ToLower(p.name), strings.ToLower(toComplete)) {
			matches = append(matches, p.name+"\t"+p.desc)
		}
	}
	return matches
}

// CompleteConfigKeys provides dynamic tab-completion for configurable parameter names.
func CompleteConfigKeys(toComplete string) []string {
	keys := []struct {
		name string
		desc string
	}{
		{"attachment_save_dir", "Attachment download directory"},
		{"auto_unsubscribe", "Auto-unsubscribe from political mail"},
		{"badge", "Account tag in mixed-account listings"},
		{"block_political", "Auto-block political spam senders"},
		{"browser", "External HTML body viewer command"},
		{"default_send_account", "Default account for outbound mail"},
		{"download_dir", "Local mail cache directory"},
		{"limit", "Default maximum messages to scan or search"},
		{"llm_api_key", "API key for LLM summarization"},
		{"llm_auto_summary", "Generate AI summaries automatically"},
		{"llm_endpoint", "API URL for LLM service"},
		{"llm_max_tokens", "Maximum response tokens for LLM"},
		{"llm_model", "Model identifier for LLM service"},
		{"llm_reasoning_effort", "Thinking effort for hybrid LLMs (none/low/medium/high)"},
		{"llm_system_prompt", "System prompt for email summarization"},
		{"llm_temperature", "Sampling temperature for LLM"},
		{"mutt_aliases", "Path to Mutt aliases file"},
		{"score_threshold", "Spam probability threshold (0.0 - 1.0)"},
		{"spam_folder", "Folder name for spam messages"},
		{"spam_learn", "Folder to train spam filter on"},
		{"unspam_learn", "Folder to train ham filter on"},
		{"username", "The account's email address"},
	}
	var matches []string
	for _, k := range keys {
		if strings.HasPrefix(strings.ToLower(k.name), strings.ToLower(toComplete)) {
			matches = append(matches, k.name+"\t"+k.desc)
		}
	}
	return matches
}

// CompleteLabelRefs completes a positional that names a label.
//
// It offers labels only in the %-sigil form, which is where the program is
// going. With nothing typed it offers that form; with a sigil typed it keeps
// it, along with any "account:" after it. A bare word gets a hint instead of
// folders: bare names are the deprecated spelling, and offering them both
// taught it and buried what the word usually is in these positions -- "mt w"
// listed every folder starting with w beside the subcommand "write". The shell
// shows the hint only when nothing else matches, so "mt w" still completes to
// "write", which clihelp offers from the command tree.
//
// Flags that take a folder *name* rather than a label reference -- splice's
// -f/-F/-Y, which build a destination path -- use CompleteFolderNames directly,
// because a sigil in those is wrong.
func CompleteLabelRefs(toComplete string) []string {
	if toComplete == "" {
		return CompleteFolderNames(cfg_g.LabelSigil)
	}
	if !strings.HasPrefix(toComplete, cfg_g.LabelSigil) {
		return []string{clihelp.CompletionHint("folder names start with " + cfg_g.LabelSigil + ", as in " + cfg_g.LabelSigil + toComplete)}
	}
	return CompleteFolderNames(toComplete)
}

// CompleteFolderNames provides dynamic tab-completion for standard and cached folder names.
func CompleteFolderNames(toComplete string) []string {
	// A label is written %name, so the word being completed usually carries the
	// sigil. It is stripped before matching and put back on every candidate:
	// a shell replaces the whole token, so a candidate without it would eat the
	// %the user already typed.
	sigil := ""
	body := toComplete
	if account, label, found := splitLabelToComplete(toComplete); found {
		sigil, body = account, label
	}

	var matches []string
	prefix := strings.ToLower(body)
	standards := []string{"INBOX", "Archive", "Sent", "Drafts", "Trash", "Spam"}
	for _, s := range standards {
		if strings.HasPrefix(strings.ToLower(s), prefix) {
			matches = append(matches, sigil+s+"\tstandard folder")
		}
	}

	for _, name := range loadCachedFolderNames() {
		if !strings.HasPrefix(strings.ToLower(name), prefix) {
			continue
		}
		if containsFold(matches, sigil+name) {
			continue
		}
		matches = append(matches, sigil+name+"\tfolder")
	}
	return matches
}

// splitLabelToComplete separates the part of a word a shell must keep -- the
// sigil and any "account:" prefix -- from the label text being matched.
func splitLabelToComplete(toComplete string) (keep, label string, found bool) {
	if !strings.HasPrefix(toComplete, cfg_g.LabelSigil) {
		return "", toComplete, false
	}
	rest := toComplete[len(cfg_g.LabelSigil):]
	if idx := strings.Index(rest, ":"); idx >= 0 {
		return cfg_g.LabelSigil + rest[:idx+1], rest[idx+1:], true
	}
	return cfg_g.LabelSigil, rest, true
}

func containsFold(slice []string, val string) bool {
	for _, item := range slice {
		cand, _, _ := strings.Cut(item, "\t")
		if strings.EqualFold(cand, val) {
			return true
		}
	}
	return false
}

func loadCachedFolderNames() []string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		homeDir, hErr := os.UserHomeDir()
		if hErr != nil {
			return nil
		}
		cacheDir = filepath.Join(homeDir, ".cache")
	}
	baseDir := filepath.Join(cacheDir, app.AppName)
	paths := []string{filepath.Join(baseDir, "labels_cache.json")}
	if entries, rErr := os.ReadDir(baseDir); rErr == nil {
		for _, e := range entries {
			if e.IsDir() {
				paths = append(paths, filepath.Join(baseDir, e.Name(), "labels_cache.json"))
			}
		}
	}
	return collectLabelNamesFromPaths(paths)
}

func collectLabelNamesFromPaths(paths []string) []string {
	var names []string
	seen := make(map[string]bool)
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var items []cfg_acc.Item
		if err := json.Unmarshal(data, &items); err != nil {
			continue
		}
		for _, item := range items {
			name := item.Name
			if name == "" {
				name = item.FullName
			}
			lower := strings.ToLower(name)
			if name != "" && !seen[lower] {
				seen[lower] = true
				names = append(names, name)
			}
		}
	}
	return names
}

// AddToList adds an email to a list (whitelist or blacklist).
func AddToList(w io.Writer, targetAcc *cfg_acc.AccountConfig, field, email string) {
	switch field {
	case "whitelist":
		targetAcc.Whitelist = append(targetAcc.Whitelist, email)
		fmt.Fprintf(w, "%s Added '%s' to whitelist for account %s.\n", app.PrefixSuccess, email, targetAcc.Name)
	case "blacklist":
		targetAcc.Blacklist = append(targetAcc.Blacklist, email)
		fmt.Fprintf(w, "%s Added '%s' to blacklist for account %s.\n", app.PrefixSuccess, email, targetAcc.Name)
	}
}

// ListEntries lists entries from a list (whitelist or blacklist).
func ListEntries(w io.Writer, targetAcc *cfg_acc.AccountConfig, field string) {
	var entries []string
	switch field {
	case "whitelist":
		entries = targetAcc.Whitelist
		fmt.Fprintf(w, "Whitelist for account %s:\n", targetAcc.Name)
	case "blacklist":
		entries = targetAcc.Blacklist
		fmt.Fprintf(w, "Blacklist for account %s:\n", targetAcc.Name)
	}
	if len(entries) == 0 {
		fmt.Fprintln(w, "  (none)")
		return
	}
	for _, e := range entries {
		fmt.Fprintf(w, "  - %s\n", e)
	}
}

// RemoveFromList removes an email from a list (whitelist or blacklist).
func RemoveFromList(w io.Writer, targetAcc *cfg_acc.AccountConfig, field, email string) {
	var entries *[]string
	var name string
	switch field {
	case "whitelist":
		entries = &targetAcc.Whitelist
		name = "whitelist"
	case "blacklist":
		entries = &targetAcc.Blacklist
		name = "blacklist"
	default:
		return
	}
	for i, e := range *entries {
		if strings.EqualFold(strings.TrimSpace(e), strings.TrimSpace(email)) {
			*entries = append((*entries)[:i], (*entries)[i+1:]...)
			fmt.Fprintf(w, "%s Removed '%s' from %s for account %s.\n", app.PrefixSuccess, email, name, targetAcc.Name)
			return
		}
	}
	fmt.Fprintf(w, "%s Email '%s' is not in the %s.\n", app.PrefixWarn, email, name)
}

// Error helpers for consistent error messages

// GetValidatedClient resolves the selected account's client and reaches the
// server once to confirm the credentials work.
//
// Most commands want both, and doing them separately was five lines repeated
// across the package -- twice as an identical named helper.
func GetValidatedClient(session *Session) (mailclient.MailClient, error) {
	client, err := session.clients().ForSelected(session.Config)
	if err != nil {
		return nil, err
	}
	if err := client.Validate(); err != nil {
		return nil, err
	}
	return client, nil
}

// ErrAccountNotFound returns a standardized error for missing account.
func ErrAccountNotFound(name string) error {
	return fmt.Errorf("account %q not found in config", name)
}

// ErrAccountNotFoundAlt returns an alternative error message for missing account.
func ErrAccountNotFoundAlt(name string) error {
	return fmt.Errorf("account %q not found", name)
}

// ErrAccountExists returns a standardized error for duplicate account.
func ErrAccountExists(name string) error {
	return fmt.Errorf("an account named %q already exists", name)
}

// ErrInvalidAccountType returns a standardized error for invalid account type.
func ErrInvalidAccountType(acctType string) error {
	return fmt.Errorf("unknown account type %q (expected 'jmap', 'gmail', or 'outlook')", acctType)
}

// ErrInvalidAccountMode returns a standardized error for invalid account mode.
func ErrInvalidAccountMode(mode string) error {
	return fmt.Errorf("%w: invalid account type %q (must be 'regular' or 'test')", app.ErrUsage, mode)
}

// ErrInvalidArgs returns a standardized error for invalid arguments.
func ErrInvalidArgs(cmd, expected string) error {
	return fmt.Errorf("%w: too many arguments for '%s' (expected: %s)", app.ErrUsage, cmd, expected)
}

// SanitizeLabelCache caches sanitized label names to avoid repeated computation.
// Use this when the same label is used multiple times in a single operation.
type SanitizeLabelCache map[string]string

// Get returns the sanitized label, computing it if not already cached.
func (c SanitizeLabelCache) Get(label string) string {
	if c == nil {
		return cfg_g.SanitizeLabelForCache(label)
	}
	sanitized, ok := c[label]
	if !ok {
		sanitized = cfg_g.SanitizeLabelForCache(label)
		c[label] = sanitized
	}
	return sanitized
}

// NewSanitizeLabelCache creates a new cache for sanitized labels.
func NewSanitizeLabelCache() SanitizeLabelCache {
	return make(SanitizeLabelCache)
}

// ctxStdout is the writer clihelp gave this invocation, or standard output.
func ctxStdout(ctx *clihelp.Context) io.Writer {
	if ctx != nil && ctx.Stdout != nil {
		return ctx.Stdout
	}
	return os.Stdout
}

// ErrUsage is app.ErrUsage, so this package can wrap it without importing app
// at every site that needs to.
func ErrUsage() error { return app.ErrUsage }

// newRemoveByIDCmd builds the `rm <id>...` subcommand that deletes each ID
// named on the command line, as `msg drafts rm` and `msg outbox rm` both do.
func newRemoveByIDCmd(description, usageLine string, remove func([]string) error) clihelp.Command {
	return clihelp.Command{
		Name:        "rm",
		NoAbbrev:    true,
		Description: description,
		UsageLine:   usageLine,
		Args:        clihelp.MinimumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return remove(ctx.Args)
		},
	}
}
