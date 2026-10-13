package gmail

import (
	"context"
	"errors"
	"github.com/sarielhp/mail_cli/backend/backendtest"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/report"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestGmailClient_MessageAccessorContextCancellation(t *testing.T) {
	backendtest.AssertAccessorHonorsCancellation(t, &client{})
}

func TestGetTokenFromWebContext_Cancellation(t *testing.T) {
	config := &oauth2.Config{}
	appCfg := &Config{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := getTokenFromWebContext(ctx, report.Discard(), config, appCfg)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("getTokenFromWebContext with canceled ctx error = %v, want context.Canceled", err)
	}
}

func TestGetTokenFromWebContext_ConcurrentCancellation(t *testing.T) {
	config := &oauth2.Config{}
	appCfg := &Config{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := getTokenFromWebContext(ctx, report.Discard(), config, appCfg)
	elapsed := time.Since(start)

	if err == nil || (!errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled)) {
		t.Fatalf("getTokenFromWebContext error = %v, want deadline exceeded or canceled", err)
	}
	if elapsed > 1*time.Second {
		t.Fatalf("getTokenFromWebContext blocked for %v, want < 1s", elapsed)
	}
}

func TestGmailClient_Validate(t *testing.T) {
	t.Run("invalid account params fail immediately", func(t *testing.T) {
		client := &client{
			account: AccountConfig{
				Name: "",
			},
		}
		err := client.Validate()
		if err == nil {
			t.Fatal("expected error on invalid account params, got nil")
		}
	})

	t.Run("missing credentials fails without hanging", func(t *testing.T) {
		client := &client{
			account: AccountConfig{
				Name:     "work",
				Type:     "gmail",
				Username: "user@example.com",
				Password: "secret-password",
				IMAPHost: "imap.gmail.com",
			},
			config: &Config{
				Paths: cfg_g.Paths{ConfigDir: t.TempDir()},
			},
			rep: report.Discard(),
		}
		err := client.Validate()
		if err == nil {
			t.Fatal("expected error on missing credentials, got nil")
		}
	})
}
