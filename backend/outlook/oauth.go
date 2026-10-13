package outlook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"
)

var (
	clientID      = "14d82eec-204b-4c2f-b7e8-296a70dab67e" // Microsoft Graph Command Line Tools
	outlookScopes = "User.Read Mail.ReadWrite Mail.Send offline_access"
	tokenURL      = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
	deviceCodeURL = "https://login.microsoftonline.com/common/oauth2/v2.0/devicecode"

	oauthHTTPClient = &http.Client{Timeout: 30 * time.Second}
)

// Token is the stored OAuth credential for an Outlook account: the access and
// refresh tokens and the access token's expiry.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
}

type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
	Message         string `json:"message"`
}

type tokenResponse struct {
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int    `json:"expires_in"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Error        string `json:"error"`
}

func tokenFileName(accName string) string {
	return hashedTokenFileName(accName, cfg_g.SanitizeLabelForCache)
}

// legacyHashedTokenFileName is tokenFileName's own output computed with the
// pre-H4 sanitizer instead of the current one. A token saved under this name
// before H4, for an account whose name held a character outside
// [A-Za-z0-9_-], is not found by tokenFileName once SanitizeLabelForCache
// stops matching it -- this is what GetTokenPath's migration step looks for.
func legacyHashedTokenFileName(accName string) string {
	return hashedTokenFileName(accName, cfg_g.LegacySanitizeLabelForCache)
}

func hashedTokenFileName(accName string, sanitize func(string) string) string {
	safe := strings.Trim(sanitize(accName), "_-")
	if safe == "" {
		safe = "account"
	}
	sum := sha256.Sum256([]byte(accName))
	hash := hex.EncodeToString(sum[:8])
	return fmt.Sprintf("outlook_token_%s_%s.json", safe, hash)
}

// legacyTokenFileName is the pre-v0.6 unhashed scheme's filename, which
// predates tokenFileName's hash suffix entirely. It has to use the
// sanitizer that was in effect when those files were actually written --
// the pre-H4 one -- or GetLegacyTokenPath computes a name nothing on disk
// ever had for any account name holding a character outside
// [A-Za-z0-9_-], and migrateLegacyToken silently never finds a legacy
// token that is really there.
func legacyTokenFileName(accName string) string {
	return fmt.Sprintf("outlook_token_%s.json", cfg_g.LegacySanitizeLabelForCache(accName))
}

func resolveConfigDir(config *cfg_g.Config) (string, error) {
	if config == nil {
		return "", fmt.Errorf("config cannot be nil")
	}
	configDir := config.ConfigDir
	if configDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine outlook token location: %w", err)
		}
		configDir = filepath.Join(homeDir, ".config", app.AppName)
	}
	if !filepath.IsAbs(configDir) {
		return "", fmt.Errorf("outlook token directory %q is not absolute", configDir)
	}
	return configDir, nil
}

// GetTokenPath returns the canonical hashed token path for an account.
func GetTokenPath(config *cfg_g.Config, accName string) (string, error) {
	configDir, err := resolveConfigDir(config)
	if err != nil {
		return "", err
	}
	newPath := filepath.Join(configDir, tokenFileName(accName))
	// H4 changed what "safe" resolves to in tokenFileName for any account
	// name holding a character outside [A-Za-z0-9_-], so a token saved
	// before that fix is not this filename -- it is this one computed with
	// the old sanitizer. Move it forward once, here, rather than teaching
	// every reader a fallback lookup.
	oldPath := filepath.Join(configDir, legacyHashedTokenFileName(accName))
	cfg_g.MigrateFileSanitization(oldPath, newPath)
	return newPath, nil
}

// GetLegacyTokenPath returns the pre-v0.6 unhashed token path for backward compatibility.
func GetLegacyTokenPath(config *cfg_g.Config, accName string) (string, error) {
	configDir, err := resolveConfigDir(config)
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, legacyTokenFileName(accName)), nil
}

func loadToken(path string) (*Token, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var tok Token
	if err := json.NewDecoder(io.LimitReader(f, 1<<20)).Decode(&tok); err != nil {
		return nil, err
	}
	return &tok, nil
}

func saveToken(path string, tok *Token) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".outlook_token_*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		_ = f.Close()
		_ = os.Remove(tmp)
	}()
	if err := f.Chmod(0600); err != nil {
		return err
	}
	if err := json.NewEncoder(f).Encode(tok); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func postOAuthForm(ctx context.Context, endpoint string, values url.Values) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return oauthHTTPClient.Do(req)
}

func printDeviceCodeBanner(rep report.Reporter, dcr *deviceCodeResponse) {
	rep.Printf("\n==================================================================\n")
	rep.Printf("              OUTLOOK OAUTH DEVICE AUTHENTICATION\n")
	rep.Printf("==================================================================\n")
	rep.Printf("To sign in, please open a web browser on the page:\n")
	rep.Printf("\n    ")
	rep.Printf("%s\n", app.ColorBoldCyan.Sprint(dcr.VerificationURI))
	rep.Printf("\n")
	rep.Printf("And enter the following code: ")
	rep.Printf("%s", app.ColorBoldYellow.Sprint(dcr.UserCode))
	rep.Printf(" to authenticate.\n")
	rep.Printf("==================================================================\n")
}

func pollDeviceCodeOnce(ctx context.Context, deviceCode string, interval *time.Duration) (*Token, bool, error) {
	resp, err := postOAuthForm(ctx, tokenURL, url.Values{
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		"client_id":   {clientID},
		"device_code": {deviceCode},
	})
	if err != nil {
		return nil, true, nil
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		_ = resp.Body.Close()
	}()

	var tr tokenResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&tr); err != nil {
		return nil, true, nil
	}
	switch tr.Error {
	case "":
	case "authorization_pending":
		return nil, true, nil
	case "slow_down":
		*interval += 5 * time.Second
		return nil, true, nil
	default:
		return nil, false, fmt.Errorf("oauth flow error: %s", tr.Error)
	}

	if tr.AccessToken == "" || tr.ExpiresIn <= 0 {
		return nil, false, fmt.Errorf("oauth flow error: missing access token or expiration in response")
	}

	return &Token{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		Expiry:       time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
	}, false, nil
}

// ErrInteractiveAuthRequired reports that the account needs a sign-in a
// non-interactive process cannot perform.
var ErrInteractiveAuthRequired = mailclient.ErrInteractiveAuthRequired

// interactiveAuthPossible reports whether there is a person here to read a code
// off the screen and type it into a browser.
//
// It is a variable so a test can drive both answers without a terminal.
var interactiveAuthPossible = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// getNewTokenViaDeviceCode signs in by printing a URL and a code for the user
// to enter in a browser, then polling until they do.
//
// It refuses outright when nothing is attached to a terminal or when the reporter
// cannot present interactive prompts (e.g. inside TUI or cron). The flow prints a
// banner and then waits -- for the device code's lifetime, up to fifteen
// minutes -- for an action only a person can take. Reached from a cron scan or
// a test, that is fifteen minutes of a process that looks busy and is waiting
// for someone who will never arrive. The same rule as every other prompt in
// this program: with nobody to ask, fail and say what to run.
func getNewTokenViaDeviceCode(ctx context.Context, rep report.Reporter) (*Token, error) {
	if !interactiveAuthPossible() || !report.CanPrompt(rep) {
		return nil, fmt.Errorf("%w: run `mail_cli account login <name>` from a terminal (device-code sign-in needs a browser and cannot run unattended)",
			ErrInteractiveAuthRequired)
	}

	rep.Printf("[*] Requesting Microsoft Device Code...\n")
	resp, err := postOAuthForm(ctx, deviceCodeURL, url.Values{
		"client_id": {clientID},
		"scope":     {outlookScopes},
	})
	if err != nil {
		return nil, fmt.Errorf("device code request failed: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		b := readErrorBody(resp.Body)
		return nil, fmt.Errorf("device code request returned status %d: %s", resp.StatusCode, b)
	}

	var dcr deviceCodeResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&dcr); err != nil {
		return nil, fmt.Errorf("failed to decode device code response: %w", err)
	}

	printDeviceCodeBanner(rep, &dcr)

	interval := time.Duration(dcr.Interval) * time.Second
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	maxWait := time.Duration(dcr.ExpiresIn) * time.Second
	if maxWait <= 0 || maxWait > 15*time.Minute {
		maxWait = 15 * time.Minute
	}
	expiry := time.Now().Add(maxWait)

	for {
		if time.Now().After(expiry) {
			return nil, fmt.Errorf("device authorization code expired")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}

		tok, retry, err := pollDeviceCodeOnce(ctx, dcr.DeviceCode, &interval)
		if err != nil {
			return nil, err
		}
		if retry {
			continue
		}
		return tok, nil
	}
}

func refreshToken(ctx context.Context, tok *Token) (*Token, error) {
	resp, err := postOAuthForm(ctx, tokenURL, url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {clientID},
		"refresh_token": {tok.RefreshToken},
		"scope":         {outlookScopes},
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		b := readErrorBody(resp.Body)
		return nil, fmt.Errorf("token refresh returned status %d: %s", resp.StatusCode, b)
	}

	var tr tokenResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&tr); err != nil {
		return nil, err
	}
	if tr.Error != "" {
		return nil, fmt.Errorf("token refresh rejected: %s", tr.Error)
	}
	if tr.AccessToken == "" || tr.ExpiresIn <= 0 {
		return nil, fmt.Errorf("token refresh returned no usable access token")
	}

	next := &Token{
		AccessToken:  tr.AccessToken,
		RefreshToken: tok.RefreshToken,
		Expiry:       time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
	}
	if tr.RefreshToken != "" {
		next.RefreshToken = tr.RefreshToken
	}
	return next, nil
}

type tokenSource struct {
	tokenPath  string
	legacyPath string
	initErr    error
	mu         sync.Mutex
	token      *Token

	// rep carries the device-code sign-in instructions, which the user has to
	// read for the flow to complete.
	rep report.Reporter
}

func newTokenSource(config *cfg_g.Config, accName string) *tokenSource {
	path, err := GetTokenPath(config, accName)
	legacyPath, _ := GetLegacyTokenPath(config, accName)
	return &tokenSource{
		tokenPath:  path,
		legacyPath: legacyPath,
		initErr:    err,
	}
}

func (ts *tokenSource) Token() (*Token, error) {
	return ts.TokenContext(context.Background())
}

func migrateLegacyToken(tokenPath, legacyPath string) (*Token, error) {
	if legacyPath == "" || legacyPath == tokenPath {
		return nil, os.ErrNotExist
	}
	tok, err := loadToken(legacyPath)
	if err != nil {
		return nil, err
	}
	if err := saveToken(tokenPath, tok); err != nil {
		return nil, fmt.Errorf("failed to migrate legacy outlook token to secure storage: %w", err)
	}
	if err := os.Remove(legacyPath); err != nil {
		return nil, fmt.Errorf("migrated outlook token but failed to remove insecure legacy copy %q: %w", legacyPath, err)
	}
	return tok, nil
}

func (ts *tokenSource) TokenContext(ctx context.Context) (*Token, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.initErr != nil {
		return nil, fmt.Errorf("invalid token configuration: %w", ts.initErr)
	}

	if ts.token == nil {
		tok, err := loadToken(ts.tokenPath)
		if err != nil && (os.IsNotExist(err) || errors.Is(err, os.ErrNotExist)) {
			tok, err = migrateLegacyToken(ts.tokenPath, ts.legacyPath)
			if err != nil && !os.IsNotExist(err) && !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
		}
		if err != nil {
			tok, err = getNewTokenViaDeviceCode(ctx, report.Or(ts.rep))
			if err != nil {
				return nil, err
			}
			if err := saveToken(ts.tokenPath, tok); err != nil {
				return nil, fmt.Errorf("failed to persist initial outlook token: %w", err)
			}
		}
		ts.token = tok
	}

	if time.Now().Add(1 * time.Minute).After(ts.token.Expiry) {
		tok, err := refreshToken(ctx, ts.token)
		if err != nil {
			tok, err = getNewTokenViaDeviceCode(ctx, report.Or(ts.rep))
			if err != nil {
				return nil, err
			}
		}
		ts.token = tok
		if err := saveToken(ts.tokenPath, tok); err != nil {
			return nil, fmt.Errorf("failed to persist refreshed outlook token: %w", err)
		}
	}

	return ts.token, nil
}

type oauthTransport struct {
	base        http.RoundTripper
	tokenSource *tokenSource
}

func (t *oauthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL == nil || !strings.EqualFold(req.URL.Hostname(), "graph.microsoft.com") {
		var host string
		if req.URL != nil {
			host = req.URL.Host
		}
		return nil, fmt.Errorf("refusing to attach outlook credential to non-Graph host %q", host)
	}
	tokCtx, cancel := context.WithTimeout(context.WithoutCancel(req.Context()), 15*time.Minute)
	defer cancel()

	tok, err := t.tokenSource.TokenContext(tokCtx)
	if err != nil {
		return nil, err
	}
	cloned := req.Clone(req.Context())
	cloned.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(cloned)
}

func getOutlookClient(config *cfg_g.Config, accName string, rep report.Reporter) *http.Client {
	ts := newTokenSource(config, accName)
	ts.rep = report.Or(rep)
	return &http.Client{
		Transport: &oauthTransport{tokenSource: ts},
		Timeout:   30 * time.Second,
	}
}
