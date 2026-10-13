package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
)

func parseExtNoZero(args []string, isExt, isNoZero bool) (bool, bool) {
	for _, arg := range args {
		if strings.EqualFold(arg, "ext") {
			isExt = true
		} else if strings.EqualFold(arg, "nozero") {
			isNoZero = true
		}
	}
	return isExt, isNoZero
}

func cfgResolveAccount(session *Session) (*cfg_g.FileConfig, *cfg_acc.AccountConfig, string, string, error) {
	resolved, err := cfg_g.Resolve(session.Config)
	if err != nil {
		return nil, nil, "", "", err
	}
	return resolved.File, resolved.Account, "", resolved.Path, nil
}

func cfgSaveConfig(configPath string, fc *cfg_g.FileConfig) error {
	return cfg_g.SaveConfigFile(configPath, fc)
}

func updateRuleLabels(w io.Writer, targetAcc *cfg_acc.AccountConfig, oldName, newName string) {
	for i, rule := range targetAcc.Rules {
		if strings.EqualFold(rule.Label, oldName) {
			targetAcc.Rules[i].Label = newName
			fmt.Fprintf(w, "%s Updated routing rule '%s' to reference renamed label %q\n", app.PrefixSuccess, rule.Sender, newName)
		}
	}
}

// ResolveCacheDir returns the cache directory path for the current account.
func ResolveCacheDir(config *cfg_g.Config) (cachedir.Account, error) {
	resolved, err := cfg_g.Resolve(config)
	if err != nil {
		return cachedir.Account{}, err
	}
	return config.AccountCacheFor(*resolved.Account), nil
}

// SearchLabels searches cached labels matching all given substring patterns.
func SearchLabels(config *cfg_g.Config, patterns []string) ([]string, error) {
	cacheDir, err := ResolveCacheDir(config)
	if err != nil {
		return nil, err
	}
	cs := &cache.DiskCacheStore{Dir: cacheDir}
	items, cacheErr := cs.GetLabelItems()
	_, ageErr := cs.CachedLabelItemsAge()

	if cacheErr != nil {
		return nil, fmt.Errorf("labels cache not available: %w", cacheErr)
	}
	if ageErr != nil {
		return nil, fmt.Errorf("labels cache not available: %w", ageErr)
	}

	var lowerPatterns []string
	for _, p := range patterns {
		lowerPatterns = append(lowerPatterns, strings.ToLower(p))
	}

	var matches []string
	for _, item := range items {
		lowerName := strings.ToLower(item.FullName)
		allMatched := true
		for _, lp := range lowerPatterns {
			if !strings.Contains(lowerName, lp) {
				allMatched = false
				break
			}
		}
		if allMatched {
			matches = append(matches, item.FullName)
		}
	}
	sort.Strings(matches)
	return matches, nil
}

// ResolveUniqueMatch takes a search pattern and a list of matching candidates.
func ResolveUniqueMatch(pattern string, candidates []string) string {
	for _, c := range candidates {
		if strings.EqualFold(c, pattern) {
			return c
		}
	}
	if len(candidates) == 1 {
		return candidates[0]
	}
	return ""
}

func runLabelsListFull(session *Session, isExt bool, isNoZero bool, realOnly bool) error {
	client, err := GetValidatedClient(session)
	if err != nil {
		return err
	}

	var items []cfg_acc.Item
	if realOnly {
		items, err = getLabelItemsReal(client)
	} else {
		items, err = client.GetLabelItems()
	}
	if err != nil {
		return err
	}

	hideZero := isNoZero || !flagLabelsListAll
	if client.Config() != nil && client.Config().HideZeroLabels && !flagLabelsListAll {
		hideZero = true
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].FullName < items[j].FullName
	})

	for _, item := range items {
		if (hideZero || isNoZero) && item.MessagesTotal == 0 {
			continue
		}
		if isExt {
			fmt.Fprintf(session.out(), "%s (%d/%d)\n", item.FullName, item.MessagesUnread, item.MessagesTotal)
		} else {
			fmt.Fprintln(session.out(), item.FullName)
		}
	}
	return nil
}

func getLabelItemsReal(client mailclient.Reader) ([]cfg_acc.Item, error) {
	if cc, ok := client.(*mailclient.CheckingMailClient); ok {
		items, err := cc.Delegate.GetLabelItems()
		if err != nil {
			return nil, fmt.Errorf("server error fetching labels: %w", err)
		}
		return items, nil
	}
	items, err := client.GetLabelItems()
	if err != nil {
		return nil, fmt.Errorf("server error fetching labels: %w", err)
	}
	return items, nil
}

func findArg(args []string, target string) bool {
	for _, a := range args {
		if strings.EqualFold(a, target) {
			return true
		}
	}
	return false
}

func findLabelItem(items []cfg_acc.Item, target string) (*cfg_acc.Item, error) {
	targetTrimmed := strings.TrimSpace(target)
	if targetTrimmed == "" {
		return nil, fmt.Errorf("label name cannot be empty")
	}

	// 1. Direct case-insensitive match on FullName or Name
	for i := range items {
		if strings.EqualFold(items[i].FullName, targetTrimmed) || strings.EqualFold(items[i].Name, targetTrimmed) {
			return &items[i], nil
		}
	}

	// 2. Suffix / prefix match
	targetLower := strings.ToLower(targetTrimmed)
	suffix := "/" + targetLower
	var candidates []*cfg_acc.Item
	for i := range items {
		fullLower := strings.ToLower(items[i].FullName)
		if strings.HasSuffix(fullLower, suffix) || strings.HasPrefix(fullLower, targetLower) {
			candidates = append(candidates, &items[i])
		}
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	if len(candidates) > 1 {
		return nil, formatAmbiguousLabelItemError(targetTrimmed, candidates)
	}

	// 3. Substring match
	for i := range items {
		if strings.Contains(strings.ToLower(items[i].FullName), targetLower) {
			candidates = append(candidates, &items[i])
		}
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	if len(candidates) > 1 {
		return nil, formatAmbiguousLabelItemError(targetTrimmed, candidates)
	}

	return nil, fmt.Errorf("label %q not found", targetTrimmed)
}

func formatAmbiguousLabelItemError(target string, candidates []*cfg_acc.Item) error {
	var names []string
	seen := make(map[string]bool)
	for _, c := range candidates {
		if !seen[c.FullName] {
			seen[c.FullName] = true
			names = append(names, c.FullName)
		}
	}
	sort.Strings(names)
	return fmt.Errorf("ambiguous label %q (candidates: %s)", target, strings.Join(names, ", "))
}
