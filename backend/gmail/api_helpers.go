package gmail

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/report"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/cache/msg"
	"sync"

	gmailapi "google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
)

// IsInsufficientScopeError reports whether err indicates an insufficient OAuth
// scope.
func IsInsufficientScopeError(err error) bool {
	if err == nil {
		return false
	}
	if gErr, ok := err.(*googleapi.Error); ok {
		for _, e := range gErr.Errors {
			if e.Reason == "insufficientPermissions" || e.Reason == "forbidden" {
				return true
			}
		}
	}
	return false
}

// HandleScopeError deletes the cached token when err is an insufficient-scope
// error and returns a formatted error; any other error is returned unchanged.
func HandleScopeError(rep report.Reporter, config *Config, err error) error {
	if IsInsufficientScopeError(err) {
		configDir := config.ConfigDir
		if configDir == "" {
			homeDir, errHome := os.UserHomeDir()
			if errHome == nil {
				configDir = filepath.Join(homeDir, ".config", app.AppName)
			}
		}
		if configDir != "" {
			tokenPath := GetTokenPath(config)
			_ = os.Remove(tokenPath)
			rep.Printf("\n%s Error: Insufficient authentication scopes to manage Gmail filters.\n", app.PrefixError)
			rep.Infof("Deleted outdated token from %s\n", tokenPath)
			rep.Infof("Please run the command again to re-authenticate with the required settings scopes.\n")
		} else {
			rep.Printf("\n%s Error: Insufficient authentication scopes to manage Gmail filters.\n", app.PrefixError)
		}
		return fmt.Errorf("insufficient authentication scopes")
	}
	return err
}

// GetTokenPath computes the OAuth token file path, supporting account-specific
// tokens.
func GetTokenPath(config *Config) string {
	configDir := config.ConfigDir
	if configDir == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			configDir = filepath.Join(homeDir, ".config", app.AppName)
		}
	}
	tokenName := "token.json"
	if config.SelectedAccount != "" {
		tokenName = fmt.Sprintf("token_%s.json", cfg_g.SanitizeLabelForCache(string(config.SelectedAccount)))
		// H4 changed what this filename resolves to for any account name
		// holding a character outside [A-Za-z0-9_-]; move a token saved
		// under the old name forward rather than leave it looking missing
		// and forcing an unnecessary re-authentication.
		oldName := fmt.Sprintf("token_%s.json", cfg_g.LegacySanitizeLabelForCache(string(config.SelectedAccount)))
		cfg_g.MigrateFileSanitization(filepath.Join(configDir, oldName), filepath.Join(configDir, tokenName))
	}
	return filepath.Join(configDir, tokenName)
}

// DecodeGmailRaw decodes Gmail's raw message Base64 string, retrying with the
// URL and standard alphabets after stripping whitespace.
func DecodeGmailRaw(rawStr string) ([]byte, error) {
	dec, err := base64.RawURLEncoding.DecodeString(rawStr)
	if err == nil {
		return dec, nil
	}

	cleaned := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, rawStr)

	for _, enc := range []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding, base64.StdEncoding} {
		dec, err = enc.DecodeString(cleaned)
		if err == nil {
			return dec, nil
		}
	}

	return nil, fmt.Errorf("failed to decode base64 after %d attempts", len(rawStr))
}

// resolveLabelIDByName resolves a label name to its corresponding Gmail Label ID.
func resolveLabelIDByName(srv *gmailapi.Service, config *Config, labelName string) (string, error) {
	return resolveLabelIDByNameContext(context.Background(), srv, config, labelName)
}

// resolveLabelIDByNameContext resolves a label name to its corresponding Gmail Label ID respecting ctx.
func resolveLabelIDByNameContext(ctx context.Context, srv *gmailapi.Service, _ *Config, labelName string) (string, error) {
	if strings.EqualFold(labelName, "inbox") {
		return "INBOX", nil
	}
	if strings.EqualFold(labelName, "spam") {
		return "SPAM", nil
	}
	if strings.EqualFold(labelName, "trash") {
		return "TRASH", nil
	}
	labelsRes, err := srv.Users.Labels.List("me").Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("failed to list Gmail labels: %w", err)
	}
	for _, l := range labelsRes.Labels {
		if strings.EqualFold(l.Name, labelName) {
			return l.Id, nil
		}
	}
	return "", fmt.Errorf("label %q not found on Gmail", labelName)
}

// downloadMissingSpamEmails fetches and caches missing emails concurrently from Gmail Spam folder.
func downloadMissingSpamEmails(rep report.Reporter, srv *gmailapi.Service, config *Config, missingIDs []string, purpose string) error {
	return downloadMissingSpamEmailsWithContext(context.Background(), rep, srv, config, missingIDs, purpose)
}

// downloadMissingSpamEmailsWithContext fetches and caches missing emails concurrently with context support.
func downloadMissingSpamEmailsWithContext(ctx context.Context, rep report.Reporter, srv *gmailapi.Service, config *Config, missingIDs []string, purpose string) error {
	if len(missingIDs) == 0 {
		return nil
	}

	rep.Infof("Downloading %d missing spam emails for %s...\n", len(missingIDs), purpose)

	sem := make(chan struct{}, 8)
	errChan := make(chan error, len(missingIDs))
	var wg sync.WaitGroup

	var mu sync.Mutex
	count := 0
	total := len(missingIDs)

	for i, id := range missingIDs {
		wg.Add(1)
		go func(msgID string, _ int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// Check context before starting each download
			select {
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			default:
			}

			resMsg, err := srv.Users.Messages.Get("me", msgID).Format("raw").Do()
			if err != nil {
				errChan <- fmt.Errorf("failed to download message %s: %w", msgID, err)
				return
			}

			rawBytes, err := DecodeGmailRaw(resMsg.Raw)
			if err != nil {
				errChan <- fmt.Errorf("failed to decode message %s: %w", msgID, err)
				return
			}

			if err := msg.Store(config.AccountCache, email.ID(msgID), rawBytes, time.Now()); err != nil {
				errChan <- fmt.Errorf("failed to store message %s to cache: %w", msgID, err)
				return
			}

			mu.Lock()
			count++
			current := count
			mu.Unlock()

			if config.Verbose {
				rep.Successf("    Cached spam email ID %s locally\n", msgID)
			} else {
				rep.Progress(current, total, "Downloading spam emails")
			}
		}(id, i)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			return err
		}
	}
	if !config.Verbose {
		rep.Printf("\n")
	}
	return nil
}
