package cfg_g

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/sarielhp/mail_cli/cfg_acc"
)

// AppName is the program's name, used to build the paths of its default
// configuration and cache directories and the prefixes of its temporary files.
const (
	AppName = "mail_cli"
)

// evaluateDirty audits file-based config structures, applying default parameters where necessary and reporting if changes were made.
func evaluateRawFieldDefaults(fc *FileConfig, raw map[string]interface{}, config *Config) bool {
	dirty := false
	if _, ok := raw["imap_host"]; !ok || fc.IMAPHost == "" {
		fc.IMAPHost = config.IMAPHost
		dirty = true
	}
	if _, ok := raw["download_dir"]; !ok || fc.DownloadDir == "" || fc.DownloadDir == "./downloaded_emails" {
		fc.DownloadDir = config.CacheRoot
		dirty = true
	}
	if _, ok := raw["limit"]; !ok || fc.Limit == nil {
		val := config.Limit
		fc.Limit = &val
		dirty = true
	}
	if _, ok := raw["score_threshold"]; !ok || fc.ScoreThreshold == nil {
		val := config.ScoreThreshold
		fc.ScoreThreshold = &val
		dirty = true
	}
	if _, ok := raw["spam_folder"]; !ok || fc.SpamFolder == "" {
		fc.SpamFolder = config.SpamFolder
		dirty = true
	}
	if _, ok := raw["received_folder"]; !ok || fc.ReceivedFolder == "" {
		fc.ReceivedFolder = config.ReceivedFolder
		dirty = true
	}
	if _, ok := raw["spam_learn"]; !ok || fc.SpamLearn == "" {
		fc.SpamLearn = config.SpamLearn
		dirty = true
	}
	if _, ok := raw["unspam_learn"]; !ok || fc.UnspamLearn == "" {
		fc.UnspamLearn = config.UnspamLearn
		dirty = true
	}
	if _, ok := raw["allowed_languages"]; !ok || fc.AllowedLanguages == nil {
		val := config.AllowedLanguages
		fc.AllowedLanguages = &val
		dirty = true
	}
	if _, ok := raw["block_political"]; !ok || fc.BlockPolitical == nil {
		val := config.BlockPolitical
		fc.BlockPolitical = &val
		dirty = true
	}
	if _, ok := raw["auto_unsubscribe"]; !ok || fc.AutoUnsubscribe == nil {
		val := config.AutoUnsubscribe
		fc.AutoUnsubscribe = &val
		dirty = true
	}
	if _, ok := raw["editor"]; !ok || fc.Editor == "" {
		fc.Editor = config.Editor
		dirty = true
	}
	if _, ok := raw["editor_args"]; !ok || fc.EditorArgs == nil {
		fc.EditorArgs = config.EditorArgs
		dirty = true
	}
	if _, ok := raw["accounts"]; !ok || fc.Accounts == nil {
		val := []cfg_acc.AccountConfig{}
		fc.Accounts = &val
		dirty = true
	}
	return dirty
}

func evaluateAccountDefaults(fc *FileConfig) bool {
	dirty := false
	if len(*fc.Accounts) == 0 {
		*fc.Accounts = []cfg_acc.AccountConfig{
			{
				Name:           "default",
				Type:           "gmail",
				Username:       fc.Username,
				Password:       fc.Password,
				IMAPHost:       fc.IMAPHost,
				SpamFolder:     fc.SpamFolder,
				ReceivedFolder: fc.ReceivedFolder,
				SpamLearn:      fc.SpamLearn,
				UnspamLearn:    fc.UnspamLearn,
			},
		}
		dirty = true
	}

	for i := range *fc.Accounts {
		acc := &(*fc.Accounts)[i]
		if fillFolderDefaults(acc) {
			dirty = true
		}
		if acc.SpamLearn == "" {
			if strings.EqualFold(acc.Type, "gmail") {
				if acc.SpamFolder != "" {
					acc.SpamLearn = acc.SpamFolder
				} else {
					acc.SpamLearn = "[Gmail]/Spam"
				}
			} else {
				acc.SpamLearn = "LearnSpam"
			}
			dirty = true
		}
		if acc.UnspamLearn == "" {
			acc.UnspamLearn = "LearnUnSpam"
			dirty = true
		}
	}
	return dirty
}

// fillFolderDefaults gives an account with no spam or received folder its
// type's, as `account new` would have. Without them ValidateAccountParams
// refuses the account, and it did so one field at a time -- so an account
// written by an older version, or by hand, failed `account login` once per
// missing folder.
func fillFolderDefaults(acc *cfg_acc.AccountConfig) bool {
	spam, received, ok := cfg_acc.FolderDefaults(acc.Type)
	if !ok {
		return false
	}
	filled := false
	if acc.SpamFolder == "" {
		acc.SpamFolder = spam
		filled = true
	}
	if acc.ReceivedFolder == "" {
		acc.ReceivedFolder = received
		filled = true
	}
	return filled
}

func evaluateCalendarManager(accounts []cfg_acc.AccountConfig) bool {
	calendarManagerCount := 0
	gmailCount := 0
	gmailIndex := -1
	for i, acc := range accounts {
		if acc.CalendarManager {
			calendarManagerCount++
		}
		if strings.EqualFold(acc.Type, "gmail") {
			gmailCount++
			gmailIndex = i
		}
	}

	if calendarManagerCount == 0 {
		if gmailCount == 1 {
			accounts[gmailIndex].CalendarManager = true
			return true
		}
		return false
	}
	if calendarManagerCount > 1 {
		foundFirst := false
		dirty := false
		for i := range accounts {
			if accounts[i].CalendarManager {
				if !foundFirst {
					foundFirst = true
				} else {
					accounts[i].CalendarManager = false
					dirty = true
				}
			}
		}
		return dirty
	}
	return false
}

func migrateGlobalEntities(fc *FileConfig) bool {
	hasGlobalEntities := (fc.Whitelist != nil && len(*fc.Whitelist) > 0) ||
		(fc.Blacklist != nil && len(*fc.Blacklist) > 0) ||
		(fc.Rules != nil && len(*fc.Rules) > 0)
	if !hasGlobalEntities {
		return false
	}

	fmt.Printf("[*] Migrating global rules/whitelist/blacklist to account-level...\n")
	for i := range *fc.Accounts {
		acc := &(*fc.Accounts)[i]
		if fc.Whitelist != nil && len(*fc.Whitelist) > 0 {
			acc.Whitelist = append(acc.Whitelist, *fc.Whitelist...)
		}
		if fc.Blacklist != nil && len(*fc.Blacklist) > 0 {
			acc.Blacklist = append(acc.Blacklist, *fc.Blacklist...)
		}
		if fc.Rules != nil && len(*fc.Rules) > 0 {
			acc.Rules = append(acc.Rules, *fc.Rules...)
		}
	}
	fc.Whitelist = nil
	fc.Blacklist = nil
	fc.Rules = nil
	return true
}

// evaluateDirty audits file-based config structures, applying default parameters where necessary and reporting if changes were made.
func evaluateDirty(fc *FileConfig, raw map[string]interface{}, config *Config) bool {
	dirty := evaluateRawFieldDefaults(fc, raw, config)
	if evaluateAccountDefaults(fc) {
		dirty = true
	}
	if evaluateCalendarManager(*fc.Accounts) {
		dirty = true
	}
	if migrateGlobalEntities(fc) {
		dirty = true
	}
	return dirty
}

func newDefaultConfig(homeDir string) *Config {
	cacheDir, errCache := os.UserCacheDir()
	var defaultDownloadDir string
	if errCache == nil {
		defaultDownloadDir = filepath.Join(cacheDir, AppName)
	} else {
		defaultDownloadDir = filepath.Join(homeDir, ".cache", AppName)
	}

	return &Config{
		SaveSentCopy: true,
		Credentials: Credentials{
			IMAPHost: "imap.gmail.com:993",
			Username: os.Getenv("GMAIL_USER"),
			Password: os.Getenv("GMAIL_PASS"),
		},
		Paths: Paths{CacheRoot: defaultDownloadDir},
		Policy: Policy{
			Limit:            1000,
			ScoreThreshold:   0.0,
			LearnSpam:        false,
			ForceLearn:       false,
			SpamFolder:       "[Gmail]/Spam",
			ReceivedFolder:   "received",
			SpamLearn:        "LearnSpam",
			UnspamLearn:      "LearnUnSpam",
			AllowedLanguages: []string{"english", "hebrew", "german", "french"},
			BlockPolitical:   true,
			AutoUnsubscribe:  false,
			Whitelist:        []string{},
			Blacklist:        []string{},
			Rules:            []cfg_acc.Rule{},
		},
		Prefs: Prefs{
			Verbose:    false,
			Editor:     "emacs",
			EditorArgs: []string{"-nw"},
		},
	}
}

func loadOrInitFileConfig(configPath string, config *Config, noWrite bool) (*FileConfig, error) {
	configExists := true
	if _, errStat := os.Stat(configPath); os.IsNotExist(errStat) {
		configExists = false
	}

	var fc FileConfig
	dirty := false

	if !configExists {
		fc = FileConfig{
			Username:         "your-email@gmail.com",
			Password:         "your-app-password",
			IMAPHost:         config.IMAPHost,
			DownloadDir:      config.CacheRoot,
			Limit:            &config.Limit,
			ScoreThreshold:   &config.ScoreThreshold,
			SpamFolder:       config.SpamFolder,
			ReceivedFolder:   config.ReceivedFolder,
			SpamLearn:        config.SpamLearn,
			UnspamLearn:      config.UnspamLearn,
			AllowedLanguages: &config.AllowedLanguages,
			BlockPolitical:   &config.BlockPolitical,
			AutoUnsubscribe:  &config.AutoUnsubscribe,
			Accounts:         &config.Accounts,
		}
		// Run the same auto-fix pass the verifier below will re-run on
		// reload -- not just evaluateAccountDefaults -- so the bytes this
		// writes already match what evaluateDirty expects to find. Seeding
		// only the account (an earlier version of this fix) left
		// evaluateCalendarManager's own auto-fix unseeded: a freshly
		// created single gmail account always gets CalendarManager flipped
		// on by that pass, so the verifier still found the reloaded config
		// dirty and a first run still failed, just from a different field
		// than "accounts": null.
		evaluateDirty(&fc, map[string]interface{}{}, config)
		dirty = true
	} else {
		data, errRead := os.ReadFile(configPath)
		if errRead != nil {
			return nil, fmt.Errorf("error reading config file %s: %w", configPath, errRead)
		}
		loadedFc, errLoad := LoadConfigFile(configPath)
		if errLoad != nil {
			return nil, fmt.Errorf("error loading config: %w", errLoad)
		}
		fc = *loadedFc

		var raw map[string]interface{}
		if err := json.Unmarshal(data, &raw); err != nil {
			slog.Warn("failed to parse config for dirty check", slog.Any("error", err))
		}
		if raw == nil {
			raw = make(map[string]interface{})
		}
		dirty = evaluateDirty(&fc, raw, config)
	}

	// A --read-only or --dry-run invocation must not mutate config.json
	// before flags are even parsed (L3): an in-memory default or auto-fixed
	// value is enough to run this one command, and writing it to disk is
	// exactly the kind of local write that would record something the run
	// never asked to happen. The next ordinary run persists it as usual.
	if !dirty || noWrite {
		return &fc, nil
	}

	if errSave := SaveConfigFile(configPath, &fc); errSave != nil {
		return nil, fmt.Errorf("failed to write config file: %w", errSave)
	}

	verifyFc, errReload := LoadConfigFile(configPath)
	if errReload != nil {
		return nil, fmt.Errorf("failed to reload config for verification: %w", errReload)
	}

	verifyData, errReadVerify := os.ReadFile(configPath)
	if errReadVerify != nil {
		return nil, fmt.Errorf("failed to read config for verification: %w", errReadVerify)
	}
	var verifyRaw map[string]interface{}
	if err := json.Unmarshal(verifyData, &verifyRaw); err != nil {
		slog.Warn("failed to parse verified config", slog.Any("error", err))
	}
	if verifyRaw == nil {
		verifyRaw = make(map[string]interface{})
	}

	if evaluateDirty(verifyFc, verifyRaw, config) {
		return nil, fmt.Errorf("config update verification failed: configuration remains dirty after writing defaults")
	}

	if !configExists {
		fmt.Printf("[*] Config file automatically created at: %s\n", configPath)
		fmt.Printf("[*] Please configure your credentials inside this file before running again.\n")
	} else {
		fmt.Printf("[*] Updated config file at %s with missing defaults\n", configPath)
	}
	return verifyFc, nil
}

func applyFileNetworkAndFolderConfig(config *Config, fc *FileConfig) {
	if fc.Username != "" {
		config.Username = fc.Username
	}
	if fc.Password != "" {
		config.Password = fc.Password
	}
	if fc.IMAPHost != "" {
		config.IMAPHost = fc.IMAPHost
	}
	if fc.DownloadDir != "" {
		config.CacheRoot = fc.DownloadDir
	}
	if fc.MuttAliases != "" {
		config.MuttAliases = fc.MuttAliases
	}
	if fc.DefaultSendAccount != "" {
		config.DefaultSendAccount = fc.DefaultSendAccount
	}
	if fc.Limit != nil {
		config.Limit = *fc.Limit
	}
	if fc.ScoreThreshold != nil {
		config.ScoreThreshold = *fc.ScoreThreshold
	}
	if fc.SpamFolder != "" {
		config.SpamFolder = fc.SpamFolder
	}
	if fc.ReceivedFolder != "" {
		config.ReceivedFolder = fc.ReceivedFolder
	}
	config.SpamLearn = "LearnSpam"
	if fc.SpamLearn != "" {
		config.SpamLearn = fc.SpamLearn
	}
	config.UnspamLearn = "LearnUnSpam"
	if fc.UnspamLearn != "" {
		config.UnspamLearn = fc.UnspamLearn
	}
}

func applyFileFilterAndToolConfig(config *Config, fc *FileConfig) {
	if fc.AllowedLanguages != nil {
		config.AllowedLanguages = *fc.AllowedLanguages
	}
	if fc.BlockPolitical != nil {
		config.BlockPolitical = *fc.BlockPolitical
	}
	if fc.AutoUnsubscribe != nil {
		config.AutoUnsubscribe = *fc.AutoUnsubscribe
	}
	if fc.Accounts != nil {
		config.Accounts = *fc.Accounts
	}
	config.Browser = fc.Browser
	if fc.Editor != "" {
		config.Editor = fc.Editor
	}
	if fc.EditorArgs != nil {
		config.EditorArgs = fc.EditorArgs
	}
	if fc.ReadOnly != nil {
		config.ReadOnly = *fc.ReadOnly
	}
	if fc.LLM != nil {
		config.LLM = *fc.LLM
	}
	if fc.AttachmentSaveDir != "" {
		config.AttachmentSaveDir = fc.AttachmentSaveDir
	}
	if fc.AttachmentInvertColors != nil {
		config.AttachmentInvertColors = *fc.AttachmentInvertColors
	}
	if fc.SaveSentCopy != nil {
		config.SaveSentCopy = *fc.SaveSentCopy
	}
	if fc.GitHubURL != "" {
		config.GitHubURL = fc.GitHubURL
	}
	if len(config.Accounts) == 0 {
		config.Accounts = []cfg_acc.AccountConfig{
			{
				Name:            "default",
				Type:            "gmail",
				Username:        config.Username,
				Password:        config.Password,
				IMAPHost:        config.IMAPHost,
				SpamFolder:      config.SpamFolder,
				ReceivedFolder:  config.ReceivedFolder,
				SpamLearn:       config.SpamFolder,
				UnspamLearn:     config.UnspamLearn,
				CalendarManager: true,
			},
		}
	}
}

func applyFileConfig(config *Config, fc *FileConfig) {
	applyFileNetworkAndFolderConfig(config, fc)
	applyFileFilterAndToolConfig(config, fc)
}

func applyEnvironmentOverrides(config *Config) {
	if envEndpoint := os.Getenv("LLM_ENDPOINT"); envEndpoint != "" {
		config.LLM.Endpoint = envEndpoint
	}
	if envKey := os.Getenv("LLM_API_KEY"); envKey != "" {
		config.LLM.APIKey = envKey
	}
	if envModel := os.Getenv("LLM_MODEL"); envModel != "" {
		config.LLM.Model = envModel
	}
	if envAutoSum := os.Getenv("LLM_AUTO_SUMMARY"); envAutoSum != "" {
		val := !strings.EqualFold(envAutoSum, "false") && envAutoSum != "0" && !strings.EqualFold(envAutoSum, "no")
		config.LLM.AutoSummary = &val
	}
	if envSaveDir := os.Getenv("ATTACHMENT_SAVE_DIR"); envSaveDir != "" {
		config.AttachmentSaveDir = envSaveDir
	}
	if envInvert := os.Getenv("ATTACHMENT_INVERT_COLORS"); envInvert != "" {
		config.AttachmentInvertColors = !strings.EqualFold(envInvert, "false") && envInvert != "0" && !strings.EqualFold(envInvert, "no")
	}
	if envSaveSent := os.Getenv("SAVE_SENT_COPY"); envSaveSent != "" {
		config.SaveSentCopy = !strings.EqualFold(envSaveSent, "false") && envSaveSent != "0" && !strings.EqualFold(envSaveSent, "no")
	}
}

func selectAccountByProgramAlias(config *Config) {
	progName := filepath.Base(os.Args[0])
	if progName == AppName || progName == "main" {
		return
	}
	for _, acc := range config.Accounts {
		for _, alias := range acc.Aliases {
			if strings.EqualFold(alias, progName) {
				config.SelectedAccount = NameOf(acc)
				return
			}
		}
	}
}

// LoadConfig reads configuration from file, overrides with environment variables, then returns config.
func LoadConfig() (*Config, error) {
	return loadConfig(false)
}

// LoadConfigNoWrite is LoadConfig but skips writing config.json back to disk
// even when it needs an auto-fix (a missing field, a freshly created file).
// The caller runs under --read-only or --dry-run and must not touch the
// user's config before its own flags are even parsed (L3); the in-memory
// defaults it returns are enough to run that one command, and the next
// ordinary run persists them as usual.
func LoadConfigNoWrite() (*Config, error) {
	return loadConfig(true)
}

func loadConfig(noWrite bool) (*Config, error) {
	homeDir, errHome := os.UserHomeDir()
	if errHome != nil {
		return nil, fmt.Errorf("failed to detect user home directory: %w", errHome)
	}

	configDir := filepath.Join(homeDir, ".config", AppName)
	configPath := filepath.Join(configDir, "config.json")
	_ = os.MkdirAll(configDir, 0700)

	config := newDefaultConfig(homeDir)
	fc, err := loadOrInitFileConfig(configPath, config, noWrite)
	if err != nil {
		return nil, err
	}

	applyFileConfig(config, fc)
	applyEnvironmentOverrides(config)
	selectAccountByProgramAlias(config)

	return config, nil
}

// listContains reports whether sender matches any entry, treating an entry that
// begins with "@" as a whole-domain pattern.
//
// The CLI validates and accepts "@domain.com" entries via
// email.IsValidEmailOrDomain, whose error message even says "must contain '@'
// and valid domain" -- but matching was full-string equality, so
// "user@example.edu" never equalled "@example.edu" and every domain entry was
// silently inert. A whitelisted domain got no protection from the spam
// heuristics and a blacklisted domain was never blocked.
func listContains(sender string, list []string) bool {
	cleanSender := strings.ToLower(strings.TrimSpace(sender))
	senderDomain := ""
	if at := strings.LastIndex(cleanSender, "@"); at >= 0 {
		senderDomain = cleanSender[at+1:]
	}
	for _, item := range list {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		if strings.HasPrefix(item, "@") {
			if senderDomain != "" && senderDomain == strings.TrimPrefix(item, "@") {
				return true
			}
			continue
		}
		if cleanSender == item {
			return true
		}
	}
	return false
}

// IsWhitelisted reports whether sender matches an entry in whitelist. An entry
// is either a full address or, written as "@example.com", a whole domain.
func IsWhitelisted(sender string, whitelist []string) bool {
	return listContains(sender, whitelist)
}

// IsBlacklisted reports whether sender matches an entry in blacklist, with the
// same entry forms as IsWhitelisted.
func IsBlacklisted(sender string, blacklist []string) bool {
	return listContains(sender, blacklist)
}
