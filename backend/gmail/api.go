package gmail

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cache/dirlock"
	"github.com/sarielhp/mail_cli/report"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gmailapi "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

var oauthScopes = []string{
	gmailapi.GmailModifyScope,
	gmailapi.GmailSendScope,
	gmailapi.MailGoogleComScope,
	gmailapi.GmailSettingsBasicScope,
	"https://www.googleapis.com/auth/calendar.events",
	"https://www.googleapis.com/auth/contacts.readonly",
}

func getOauthConfig(configDir string, scopes []string) (*oauth2.Config, error) {
	credentialsPath := filepath.Join(configDir, "credentials.json")
	if _, err := os.Stat(credentialsPath); err == nil {
		b, err := os.ReadFile(credentialsPath)
		if err != nil {
			return nil, fmt.Errorf("unable to read credentials.json: %w", err)
		}
		oauthConfig, err := google.ConfigFromJSON(b, scopes...)
		if err != nil {
			return nil, fmt.Errorf("unable to parse credentials.json: %w", err)
		}
		return oauthConfig, nil
	}

	return nil, fmt.Errorf("credentials.json not found at %s", credentialsPath)
}

// GetGmailService authenticates the user and retrieves a Gmail service client.
func GetGmailService(rep report.Reporter, config *Config) (*gmailapi.Service, error) {
	return GetGmailServiceContext(context.Background(), rep, config)
}

// GetGmailServiceContext authenticates the user and retrieves a Gmail service client respecting ctx.
func GetGmailServiceContext(ctx context.Context, rep report.Reporter, config *Config) (*gmailapi.Service, error) {
	client, err := NewOAuthHTTPClientContext(ctx, rep, config)
	if err != nil {
		return nil, err
	}

	srv, err := gmailapi.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve Gmail client: %w", err)
	}

	return srv, nil
}

// resolveConfigDir returns the configured directory, falling back to the
// default location beneath the user's home.
func resolveConfigDir(config *Config) (string, error) {
	if config.ConfigDir != "" {
		return config.ConfigDir, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home: %w", err)
	}
	return filepath.Join(homeDir, ".config", app.AppName), nil
}

// NewOAuthHTTPClient returns an HTTP client authorized for the Google scopes
// this application uses, for whichever account the config currently selects.
//
// Gmail and Google Calendar share one token and one scope list, so this is what
// calendar/google is wired with: it keeps the OAuth plumbing here, beside the
// Gmail client, without that package having to import this one.
func NewOAuthHTTPClient(rep report.Reporter, config *Config) (*http.Client, error) {
	return NewOAuthHTTPClientContext(context.Background(), rep, config)
}

// NewOAuthHTTPClientContext is NewOAuthHTTPClient, respecting ctx.
func NewOAuthHTTPClientContext(ctx context.Context, rep report.Reporter, config *Config) (*http.Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	configDir, err := resolveConfigDir(config)
	if err != nil {
		return nil, err
	}

	oauthConfig, err := getOauthConfig(configDir, oauthScopes)
	if err != nil {
		return nil, err
	}

	return getOAuthClientContext(ctx, rep, oauthConfig, GetTokenPath(config), config)
}

// getOAuthClient retrieves a token, saves it, and returns the generated client.
func getOAuthClient(rep report.Reporter, config *oauth2.Config, tokenPath string, appConfig *Config) (*http.Client, error) {
	return getOAuthClientContext(context.Background(), rep, config, tokenPath, appConfig)
}

// tokenPathLocks serializes the read-refresh-persist sequence in
// getOAuthClientContext per token file. Gmail and Google Calendar share one
// token, and a worker pool (scan/scanner.go) calls this concurrently for the
// same account: without a lock, every worker reads the same still-expired
// token before any of them writes back a refreshed one, so each performs its
// own redundant refresh round-trip -- or, if the cached token is actually
// invalid, each independently deletes the token file and starts its own
// interactive re-authentication flow.
var tokenPathLocks sync.Map // map[string]*sync.Mutex

func lockForTokenPath(tokenPath string) *sync.Mutex {
	v, _ := tokenPathLocks.LoadOrStore(tokenPath, &sync.Mutex{})
	return v.(*sync.Mutex)
}

func getOAuthClientContext(ctx context.Context, rep report.Reporter, config *oauth2.Config, tokenPath string, appConfig *Config) (*http.Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	mu := lockForTokenPath(tokenPath)
	mu.Lock()
	defer mu.Unlock()

	tok, err := tokenFromFile(tokenPath)
	if err != nil {
		tok, err = getTokenFromWebContext(ctx, rep, config, appConfig)
		if err != nil {
			return nil, err
		}
		if saveErr := saveToken(rep, tokenPath, tok); saveErr != nil {
			return nil, saveErr
		}
	}

	// Create a token source that automatically handles refreshing
	ts := config.TokenSource(ctx, tok)

	// Pre-emptively verify if the token is valid or can be successfully refreshed/re-authorized.
	// We call ts.Token() to trigger a refresh if the token is expired.
	// If refresh fails with "invalid_grant" or "token expired/revoked", we delete the token and re-auth.
	refreshed, err := ts.Token()
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "invalid_grant") || strings.Contains(errStr, "expired") || strings.Contains(errStr, "revoked") {
			rep.Warnf("Cached OAuth token is invalid, expired, or revoked (%s). Re-authenticating...\n", errStr)
			_ = os.Remove(tokenPath)
			tok, err = getTokenFromWebContext(ctx, rep, config, appConfig)
			if err != nil {
				return nil, err
			}
			if saveErr := saveToken(rep, tokenPath, tok); saveErr != nil {
				return nil, saveErr
			}
			ts = config.TokenSource(ctx, tok)
		} else {
			return nil, fmt.Errorf("token refresh failed: %w", err)
		}
	} else {
		persistRefreshed(rep, tokenPath, tok, refreshed)
	}

	client := oauth2.NewClient(ctx, ts)
	client.Timeout = 60 * time.Second
	return client, nil
}

// tokenFromFile retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

func generateOAuthState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func startOAuthCallbackServer(listener net.Listener, expectedState string, codeChan chan<- string, errChan chan<- error) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		state := r.URL.Query().Get("state")
		if subtle.ConstantTimeCompare([]byte(state), []byte(expectedState)) != 1 {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "Invalid or missing OAuth state token.")
			select {
			case errChan <- fmt.Errorf("oauth state mismatch"):
			default:
			}
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "Missing authorization code.")
			select {
			case errChan <- fmt.Errorf("received redirect without code"):
			default:
			}
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><body style='font-family: Arial, sans-serif; text-align: center; margin-top: 50px;'>"+
			"<h1>Authentication Successful!</h1><p>You can close this window now and return to the terminal.</p></body></html>")
		select {
		case codeChan <- code:
		default:
		}
	})

	srv := &http.Server{
		Handler: mux,
	}
	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			select {
			case errChan <- err:
			default:
			}
		}
	}()
	return srv
}

func printOAuthInstructions(rep report.Reporter, authURL string, port int) {
	rep.Printf("\n========================================================\n")
	rep.Printf("              GMAIL API OAUTH AUTHENTICATION            \n")
	rep.Printf("========================================================\n")
	rep.Printf("1. Open the following URL in your browser to sign in:\n")
	rep.Printf("\n")
	rep.Printf("%s\n", app.ColorBoldCyan.Sprint(authURL))
	rep.Printf("\n")
	rep.Printf("2. IF RUNNING VIA SSH: Forward port %d to your local computer first:\n", port)
	rep.Printf("   ssh -L %d:127.0.0.1:%d user@remote-host\n\n", port, port)
	rep.Printf("3. If the redirect does not open automatically, copy the URL from your\n")
	rep.Printf("   browser address bar (e.g., http://127.0.0.1:.../?code=...) and paste it below.\n")
	rep.Printf("--------------------------------------------------------\n")
	rep.Printf("Paste redirected URL or authorization code here: ")
}

func extractCodeFromInput(input string) string {
	if strings.Contains(input, "code=") {
		raw := input
		if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
			raw = "http://" + raw
		}
		if u, err := url.Parse(raw); err == nil {
			if code := u.Query().Get("code"); code != "" {
				return code
			}
		}
	}
	return input
}

func startStdinCodeReader(ctx context.Context, rep report.Reporter, codeChan chan<- string) {
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			input, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			input = strings.TrimSpace(input)
			if input == "" {
				continue
			}

			code := extractCodeFromInput(input)
			if code != "" {
				select {
				case codeChan <- code:
					return
				default:
					return
				}
			}
			rep.Errorf("Could not extract authorization code from input. Please try again: ")
		}
	}()
}

// getTokenFromWebContext requests a token from the web respecting caller context.
func getTokenFromWebContext(ctx context.Context, rep report.Reporter, config *oauth2.Config, _ *Config) (*oauth2.Token, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	// Listen on a free local port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to start local listener: %w", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	config.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d", port)

	oauthState, err := generateOAuthState()
	if err != nil {
		return nil, fmt.Errorf("failed to generate oauth state: %w", err)
	}

	srv := startOAuthCallbackServer(listener, oauthState, codeChan, errChan)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	authURL := config.AuthCodeURL(oauthState, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	printOAuthInstructions(rep, authURL, port)

	// Fallback: spawn a goroutine to read from stdin
	// Use a context to signal the goroutine to stop when we return
	stdinCtx, stdinCancel := context.WithCancel(ctx)
	defer stdinCancel()
	startStdinCodeReader(stdinCtx, rep, codeChan)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case code := <-codeChan:
		tok, err := config.Exchange(ctx, code)
		if err != nil {
			return nil, fmt.Errorf("unable to exchange code for token: %w", err)
		}
		return tok, nil
	case err := <-errChan:
		return nil, fmt.Errorf("oauth error: %w", err)
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("oauth timeout after 5 minutes")
	}
}

// persistRefreshed writes back an access token that was just renewed.
//
// The refreshed token used to be discarded: it was saved only on the
// re-authentication path, so the file kept an access token that had already
// expired and every invocation paid a refresh round-trip at startup. Worse,
// each process derived its own -- a TUI and a cron scan running together each
// spend a refresh, against a provider that rate-limits them.
//
// Failure to write is not fatal. The token in hand is valid and the operation
// the user asked for should proceed; the only cost is doing this again next
// time.
func persistRefreshed(rep report.Reporter, tokenPath string, old, refreshed *oauth2.Token) {
	if refreshed == nil || old == nil || refreshed.AccessToken == old.AccessToken {
		return
	}
	// A refresh response may omit the refresh token, meaning "keep the one you
	// have". Writing the zero value would lose the only credential that can
	// renew anything, and the next run would have to re-authenticate in a
	// browser.
	if refreshed.RefreshToken == "" {
		clone := *refreshed
		clone.RefreshToken = old.RefreshToken
		refreshed = &clone
	}
	if err := saveToken(rep, tokenPath, refreshed); err != nil {
		slog.Debug("could not cache the refreshed OAuth token",
			slog.String("path", tokenPath), slog.Any("error", err))
	}
}

// saveToken saves a token to a file path.
func saveToken(rep report.Reporter, path string, token *oauth2.Token) error {
	rep.Printf("Saving credential file to: %s\n", path)
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to encode oauth token: %w", err)
	}
	if err := dirlock.AtomicWrite(path, data, 0600); err != nil {
		return fmt.Errorf("unable to cache oauth token: %w", err)
	}
	return nil
}
