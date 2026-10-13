package cfg_g

import (
	"encoding/json"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/cfg_acc"
)

// ResolveConfigPath returns the config directory and the path to config.json
// within it. It errors when the config directory was never initialized.
func ResolveConfigPath(config *Config) (string, string, error) {
	if config.ConfigDir == "" {
		return "", "", fmt.Errorf("config directory not initialized")
	}
	configPath := filepath.Join(config.ConfigDir, "config.json")
	return config.ConfigDir, configPath, nil
}

// LoadConfigFile reads and parses config.json, then folds each account's
// separately stored whitelist, blacklist and rules back in from disk. A missing
// or malformed file is an error; a config listing no accounts yields an empty
// account list.
func LoadConfigFile(configPath string) (*FileConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("error reading config file %s: %w", configPath, err)
	}
	var fc FileConfig
	if err := json.Unmarshal(data, &fc); err != nil {
		return nil, fmt.Errorf("error parsing config file %s: %w", configPath, err)
	}

	if fc.Accounts == nil {
		empty := []cfg_acc.AccountConfig{}
		fc.Accounts = &empty
	}

	configDir := filepath.Dir(configPath)
	for i := range *fc.Accounts {
		loadAccountFromDisk(configDir, &(*fc.Accounts)[i])
	}
	return &fc, nil
}

func loadAccountFromDisk(configDir string, acc *cfg_acc.AccountConfig) {
	MigrateDirSanitization(filepath.Join(configDir, "accounts"), acc.Name)
	sanitizedName := SanitizeLabelForCache(acc.Name)
	accountsDir := filepath.Join(configDir, "accounts", sanitizedName)
	accJSONPath := filepath.Join(accountsDir, "account.json")

	if accJSONData, err := os.ReadFile(accJSONPath); err == nil {
		var diskAcc cfg_acc.AccountConfig
		if json.Unmarshal(accJSONData, &diskAcc) == nil {
			name := acc.Name
			whitelist, blacklist, rules := acc.Whitelist, acc.Blacklist, acc.Rules
			*acc = diskAcc
			acc.Name = name
			acc.Whitelist = whitelist
			acc.Blacklist = blacklist
			acc.Rules = rules
		}
	}

	if len(acc.Whitelist) == 0 {
		acc.Whitelist = loadListWithLegacyFallback(
			filepath.Join(accountsDir, "whitelist.json"),
			filepath.Join(configDir, fmt.Sprintf("%s_whitelist.json", sanitizedName)),
		)
	}
	if len(acc.Blacklist) == 0 {
		acc.Blacklist = loadListWithLegacyFallback(
			filepath.Join(accountsDir, "blacklist.json"),
			filepath.Join(configDir, fmt.Sprintf("%s_blacklist.json", sanitizedName)),
		)
	}
	if len(acc.Rules) == 0 {
		rules := loadRulesWithLegacyFallback(
			filepath.Join(accountsDir, "rules.json"),
			filepath.Join(configDir, fmt.Sprintf("%s_rules.json", sanitizedName)),
		)
		if rules != nil {
			acc.Rules = rules
		} else if acc.Rules == nil {
			acc.Rules = []cfg_acc.Rule{}
		}
	}
}

func loadListWithLegacyFallback(primaryPath, legacyPath string) []string {
	target := primaryPath
	if _, err := os.Stat(primaryPath); os.IsNotExist(err) {
		target = legacyPath
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return []string{}
	}
	var list []string
	if err := json.Unmarshal(data, &list); err != nil {
		return []string{}
	}
	return list
}

func loadRulesWithLegacyFallback(primaryPath, legacyPath string) []cfg_acc.Rule {
	target := primaryPath
	if _, err := os.Stat(primaryPath); os.IsNotExist(err) {
		target = legacyPath
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return nil
	}
	var rules []cfg_acc.Rule
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil
	}
	return rules
}

// atomicWriteFile writes data to path via a temporary file in the same
// directory followed by a rename, so a crash or a full disk can never leave a
// truncated file behind.
//
// This matters most for config.json and accounts/*/account.json: os.WriteFile
// truncates before writing, and a kill between the two left invalid JSON that
// LoadConfigFile could not parse -- with no backup, the user could not even list
// their accounts to recover. The temp file must live in the destination
// directory, because os.Rename is only atomic within one filesystem.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file for %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op once the rename succeeds

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("syncing %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file for %s: %w", path, err)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("setting mode on %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("renaming into place %s: %w", path, err)
	}
	return nil
}

// SaveConfigFile writes each account's own files, then config.json with the
// per-account detail stripped out, creating the directory if it does not exist.
func SaveConfigFile(configPath string, fc *FileConfig) error {
	configDir := filepath.Dir(configPath)
	_ = os.MkdirAll(configDir, 0700)

	// 1. Save separate files for each account
	if fc.Accounts != nil {
		for i := range *fc.Accounts {
			if err := saveAccountToDisk(configDir, &(*fc.Accounts)[i]); err != nil {
				return err
			}
		}
	}

	// 2. Clone FileConfig and strip details before saving main config.json
	return saveMainConfigFile(configPath, fc)
}

func saveAccountToDisk(configDir string, acc *cfg_acc.AccountConfig) error {
	sanitizedName := SanitizeLabelForCache(acc.Name)
	accountsDir := filepath.Join(configDir, "accounts", sanitizedName)
	_ = os.MkdirAll(accountsDir, 0700)

	accDisk := *acc
	accDisk.Whitelist = nil
	accDisk.Blacklist = nil
	accDisk.Rules = nil
	accData, err := json.MarshalIndent(accDisk, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling account %q: %w", acc.Name, err)
	}
	if err := atomicWriteFile(filepath.Join(accountsDir, "account.json"), accData, 0600); err != nil {
		return err
	}

	if err := saveAccountList(accountsDir, "whitelist.json", acc.Whitelist, fmt.Sprintf("whitelist for account %q", acc.Name)); err != nil {
		return err
	}
	if err := saveAccountList(accountsDir, "blacklist.json", acc.Blacklist, fmt.Sprintf("blacklist for account %q", acc.Name)); err != nil {
		return err
	}
	if err := saveAccountRules(accountsDir, acc.Rules, fmt.Sprintf("rules for account %q", acc.Name)); err != nil {
		return err
	}

	_ = os.Remove(filepath.Join(configDir, fmt.Sprintf("%s_whitelist.json", sanitizedName)))
	_ = os.Remove(filepath.Join(configDir, fmt.Sprintf("%s_blacklist.json", sanitizedName)))
	_ = os.Remove(filepath.Join(configDir, fmt.Sprintf("%s_rules.json", sanitizedName)))
	return nil
}

func saveAccountList(dir, filename string, list []string, label string) error {
	if list == nil {
		list = []string{}
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling %s: %w", label, err)
	}
	return atomicWriteFile(filepath.Join(dir, filename), data, 0600)
}

func saveAccountRules(dir string, rules []cfg_acc.Rule, label string) error {
	if rules == nil {
		rules = []cfg_acc.Rule{}
	}
	data, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling %s: %w", label, err)
	}
	return atomicWriteFile(filepath.Join(dir, "rules.json"), data, 0600)
}

func saveMainConfigFile(configPath string, fc *FileConfig) error {
	var fcClone FileConfig
	cloneData, err := json.Marshal(fc)
	if err != nil {
		return fmt.Errorf("failed to clone config: %w", err)
	}
	if unmarshalErr := json.Unmarshal(cloneData, &fcClone); unmarshalErr != nil {
		return fmt.Errorf("failed to restore clone config: %w", unmarshalErr)
	}

	if fcClone.Accounts != nil {
		for i := range *fcClone.Accounts {
			acc := &(*fcClone.Accounts)[i]
			*acc = cfg_acc.AccountConfig{
				Name: acc.Name,
				Type: acc.Type,
			}
		}
	}
	fcClone.Whitelist = nil
	fcClone.Blacklist = nil
	fcClone.Rules = nil

	tBytes, err := json.MarshalIndent(fcClone, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := atomicWriteFile(configPath, tBytes, 0600); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", configPath, err)
	}
	return nil
}

// Resolved is the config file, the account a command is acting on, and where
// that file lives.
//
// It replaced a five-value return whose third element, the config directory,
// was discarded at all 23 call sites.
type Resolved struct {
	File    *FileConfig
	Account *cfg_acc.AccountConfig

	// Dir is the directory holding Path. Tokens and logs live beside it.
	Dir string

	// Path is the config file itself, for saving back.
	Path string
}

// Resolve loads the config file and picks out the account to act on.
func Resolve(config *Config) (Resolved, error) {
	fc, acc, dir, path, err := resolveAccount(config)
	if err != nil {
		return Resolved{Dir: dir, Path: path}, err
	}
	return Resolved{File: fc, Account: acc, Dir: dir, Path: path}, nil
}

func resolveAccount(config *Config) (*FileConfig, *cfg_acc.AccountConfig, string, string, error) {
	configDir, configPath, err := ResolveConfigPath(config)
	if err != nil {
		return nil, nil, "", "", err
	}
	fc, err := LoadConfigFile(configPath)
	if err != nil {
		return nil, nil, "", "", err
	}
	if fc.Accounts == nil || len(*fc.Accounts) == 0 {
		return nil, nil, configDir, configPath, fmt.Errorf("no accounts configured")
	}
	// An identifier that names no account used to fall back to the first one,
	// silently. Nine packages reach an account through here and then write the
	// config back, so a typo in -A edited a different account's rules.
	targetAcc, err := Lookup(*fc.Accounts, string(config.SelectedAccount))
	if err != nil {
		return nil, nil, configDir, configPath, err
	}
	return fc, targetAcc, configDir, configPath, nil
}

// ValidateAccountParams rejects an account whose required fields are missing or
// still hold a config template's placeholder value, checking only the fields the
// account's type actually needs. Outlook is exempt from the password check
// because it authenticates with an OAuth token.
func ValidateAccountParams(acc cfg_acc.AccountConfig) error {
	if acc.Name == "" {
		return fmt.Errorf("account name is missing")
	}
	if acc.Type == "" {
		return fmt.Errorf("account type is missing")
	}
	if acc.Username == "" || acc.Username == "your-email@gmail.com" || acc.Username == "your-email@example.com" {
		return fmt.Errorf("account username is missing or contains placeholder value")
	}
	if !strings.EqualFold(acc.Type, "outlook") {
		if acc.Password == "" || acc.Password == "your-app-password" || acc.Password == "your-jmap-api-token" {
			return fmt.Errorf("account password is missing or contains placeholder value")
		}
	}
	switch {
	case strings.EqualFold(acc.Type, "gmail"):
		if acc.IMAPHost == "" {
			return fmt.Errorf("imap_host is missing")
		}
	case strings.EqualFold(acc.Type, "jmap"):
		if acc.SessionURL == "" {
			return fmt.Errorf("jmap_session_url is missing")
		}
	case strings.EqualFold(acc.Type, "outlook"):
		// Outlook only needs Username (email) and OAuth token (stored on disk).
	default:
		return fmt.Errorf("unsupported account type %q", acc.Type)
	}
	if acc.SpamFolder == "" {
		return fmt.Errorf("spam_folder is missing")
	}
	if acc.ReceivedFolder == "" {
		return fmt.Errorf("received_folder is missing")
	}
	return nil
}

// NeedsOnboarding reports whether config holds no usable account -- either none
// at all, or only accounts still carrying the placeholder credentials a freshly
// created config.json is seeded with.
//
// A brand-new install is never literally account-less: loadConfig seeds a
// "default" gmail account so the older single-account code paths still resolve
// one. That seeding is why the TUI's own "no accounts" check never fired and a
// first launch fell through to a connection attempt against
// "your-email@gmail.com". This is the check that tells a first run apart from a
// configured one, and it mirrors ValidateAccountParams's placeholder rules.
func NeedsOnboarding(config *Config) bool {
	if config == nil {
		return true
	}
	return OnlyPlaceholders(config.Accounts)
}

// OnlyPlaceholders reports whether accounts holds nothing but unconfigured
// placeholders -- such as the "default" account loadConfig seeds -- or is
// empty. Setup replaces such a list rather than appending to it, since keeping
// the placeholder first left later runs selecting it.
func OnlyPlaceholders(accounts []cfg_acc.AccountConfig) bool {
	for _, acc := range accounts {
		if !isPlaceholderAccount(acc) {
			return false
		}
	}
	return true
}

func isPlaceholderAccount(acc cfg_acc.AccountConfig) bool {
	if acc.Username == "" || acc.Username == "your-email@gmail.com" || acc.Username == "your-email@example.com" {
		return true
	}
	// Outlook authenticates through OAuth, so its password is intentionally
	// blank; a real username is enough to call it configured.
	if strings.EqualFold(acc.Type, "outlook") {
		return false
	}
	return acc.Password == "" || acc.Password == "your-app-password" || acc.Password == "your-jmap-api-token"
}

// SanitizeLabelForCache maps a label or account name to a filesystem-safe
// name segment; see cachedir.SanitizeName, which it is.
func SanitizeLabelForCache(label string) string { return cachedir.SanitizeName(label) }

// AutoBlacklistInternal adds an internal blacklist rule for the sender if not already present.
func AutoBlacklistInternal(config *Config, senderEmail string) error {
	fc, targetAcc, _, configPath, err := resolveAccount(config)
	if err != nil {
		return err
	}

	// 1. Check if the sender is already whitelisted
	if IsWhitelisted(senderEmail, targetAcc.Whitelist) {
		return nil
	}

	// 2. Determine target label for Spam Learn
	targetLabel := targetAcc.SpamLearn
	if targetLabel == "" {
		targetLabel = "LearnSpam"
	}

	// 3. Check if a rule for this sender already exists
	for _, r := range targetAcc.Rules {
		if strings.EqualFold(r.Sender, senderEmail) {
			return nil
		}
	}

	// 4. Create internal rule
	targetAcc.Rules = append(targetAcc.Rules, cfg_acc.Rule{
		Sender:   senderEmail,
		Label:    targetLabel,
		Internal: true,
	})

	fmt.Printf("[+] Auto-blacklisted sender '%s' locally (internal rule pointing to %s).\n",
		senderEmail, targetLabel)

	// 5. Save back to config
	return SaveConfigFile(configPath, fc)
}

// ExpandSavePath expands ~, %YYYY, %YY, %MM, %DD, %hh, %mm, %ss in the given directory path pattern.
func ExpandSavePath(pattern string, t time.Time) (string, error) {
	if strings.TrimSpace(pattern) == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home directory: %w", err)
		}
		return filepath.Join(homeDir, "Downloads"), nil
	}

	res := strings.TrimSpace(pattern)
	if strings.HasPrefix(res, "~/") || res == "~" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home directory: %w", err)
		}
		if res == "~" {
			res = homeDir
		} else {
			res = filepath.Join(homeDir, res[2:])
		}
	}

	if t.IsZero() {
		t = time.Now()
	}

	yyyy := fmt.Sprintf("%04d", t.Year())
	yy := fmt.Sprintf("%02d", t.Year()%100)
	mm := fmt.Sprintf("%02d", int(t.Month()))
	dd := fmt.Sprintf("%02d", t.Day())
	hh := fmt.Sprintf("%02d", t.Hour())
	minStr := fmt.Sprintf("%02d", t.Minute())
	ss := fmt.Sprintf("%02d", t.Second())

	res = strings.ReplaceAll(res, "%YYYY", yyyy)
	res = strings.ReplaceAll(res, "%YY", yy)
	res = strings.ReplaceAll(res, "%Y", yyyy)
	res = strings.ReplaceAll(res, "%MM", mm)
	res = strings.ReplaceAll(res, "%M", mm)
	res = strings.ReplaceAll(res, "%DD", dd)
	res = strings.ReplaceAll(res, "%D", dd)
	res = strings.ReplaceAll(res, "%hh", hh)
	res = strings.ReplaceAll(res, "%H", hh)
	res = strings.ReplaceAll(res, "%mm", minStr)
	res = strings.ReplaceAll(res, "%ss", ss)

	return filepath.Clean(res), nil
}

// AccountLabelSpec holds a parsed label specification with an optional account indicator.
type AccountLabelSpec struct {
	AccountIndex int    // 1-based account index (1, 2, ...), or 0 if omitted.
	AccountName  string // Account name, set when prefix is not a number (e.g. "GMail:folder").
	Label        string // Clean label path without "account:" prefix.
}

// ParseAccountLabelSpec parses a string like "GMail:inbox" or "inbox".
// If raw starts with an account name followed by a colon, the two are separated.
// Only prefixes starting with a letter are treated as account names.
// Numeric prefixes like "1:label" are rejected since account indices are fragile.
// Returns an error if a numeric prefix is used.
func ParseAccountLabelSpec(raw string) (AccountLabelSpec, error) {
	if idx := strings.Index(raw, ":"); idx > 0 {
		prefix := raw[:idx]
		if num, err := strconv.Atoi(prefix); err == nil && num > 0 {
			return AccountLabelSpec{}, fmt.Errorf("numeric account index (%d:) is no longer supported — use account name instead (e.g. %q)", num, raw[idx+1:])
		}
		// Non-numeric prefix — treat as account name if it starts with a letter
		if len(prefix) > 0 && ((prefix[0] >= 'a' && prefix[0] <= 'z') || (prefix[0] >= 'A' && prefix[0] <= 'Z')) {
			return AccountLabelSpec{
				AccountName: prefix,
				Label:       raw[idx+1:],
			}, nil
		}
	}
	return AccountLabelSpec{
		Label: raw,
	}, nil
}
