package cli

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
)

func TestIsSameAccount(t *testing.T) {
	// Same account cache despite different client wrapper instances
	c1 := &mailclient.MockMailClient{
		BackendTypeVal: "gmail",
		Cfg:            &cfg_g.Config{SelectedAccount: "", Paths: cfg_g.Paths{CacheRoot: "/cache/acc1"}, AccountCache: cachedir.At("/cache/acc1")},
	}
	c2 := &mailclient.MockMailClient{
		BackendTypeVal: "gmail",
		Cfg:            &cfg_g.Config{SelectedAccount: "", Paths: cfg_g.Paths{CacheRoot: "/cache/acc1"}, AccountCache: cachedir.At("/cache/acc1")},
	}
	if !isSameAccount(c1, c2) {
		t.Errorf("expected the same account cache to be recognized as same account even if SelectedAccount is empty")
	}

	// Different DownloadDir
	c3 := &mailclient.MockMailClient{
		BackendTypeVal: "gmail",
		Cfg:            &cfg_g.Config{SelectedAccount: "other", Paths: cfg_g.Paths{CacheRoot: "/cache/acc2"}, AccountCache: cachedir.At("/cache/acc2")},
	}
	if isSameAccount(c1, c3) {
		t.Errorf("expected different DownloadDir to not be same account")
	}

	// Empty DownloadDir, matching SelectedAccount
	c4 := &mailclient.MockMailClient{
		BackendTypeVal: "gmail",
		Cfg:            &cfg_g.Config{SelectedAccount: "work"},
	}
	c5 := &mailclient.MockMailClient{
		BackendTypeVal: "gmail",
		Cfg:            &cfg_g.Config{SelectedAccount: "WORK"},
	}
	if !isSameAccount(c4, c5) {
		t.Errorf("expected matching SelectedAccount to be recognized as same account when DownloadDir is empty")
	}

	// Empty DownloadDir, different SelectedAccount
	c6 := &mailclient.MockMailClient{
		BackendTypeVal: "gmail",
		Cfg:            &cfg_g.Config{SelectedAccount: "personal"},
	}
	if isSameAccount(c4, c6) {
		t.Errorf("expected different SelectedAccount to not be same account")
	}

	// Empty DownloadDir and SelectedAccount, matching Username
	c7 := &mailclient.MockMailClient{
		BackendTypeVal: "gmail",
		Cfg:            &cfg_g.Config{Credentials: cfg_g.Credentials{Username: "user@example.com"}},
	}
	c8 := &mailclient.MockMailClient{
		BackendTypeVal: "gmail",
		Cfg:            &cfg_g.Config{Credentials: cfg_g.Credentials{Username: "USER@example.com"}},
	}
	if !isSameAccount(c7, c8) {
		t.Errorf("expected matching Username to be recognized as same account when DownloadDir and SelectedAccount are empty")
	}

	// Different BackendType
	c9 := &mailclient.MockMailClient{
		BackendTypeVal: "outlook",
		Cfg:            &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: "/cache/acc1"}, AccountCache: cachedir.At("/cache/acc1")},
	}
	if isSameAccount(c1, c9) {
		t.Errorf("expected different BackendType to not be same account")
	}

	// Nil client checks
	if isSameAccount(nil, c1) || isSameAccount(c1, nil) || isSameAccount(nil, nil) {
		t.Errorf("expected nil clients to not be same account")
	}
}

func TestParseEmailDate_ZeroDateFallback(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()

	rawNoDate := []byte("Subject: No Date Email\r\n\r\nBody without date")
	if err := msg.Store(cachedir.At(tmpDir), "msg_no_date", rawNoDate, now); err != nil {
		t.Fatalf("failed to store message: %v", err)
	}

	em, raw, year, month, err := parseEmailDate(cachedir.At(tmpDir), "msg_no_date")
	if err != nil {
		t.Fatalf("parseEmailDate failed: %v", err)
	}
	if em == nil || len(raw) == 0 {
		t.Fatal("expected parsed email and raw bytes")
	}
	if em.EmailDate.IsZero() {
		t.Error("expected non-zero EmailDate after fallback")
	}
	if year == "0001" || month == "" {
		t.Errorf("expected current year/month fallback, got year=%q, month=%q", year, month)
	}
	expectedYear := now.Format("2006")
	if year != expectedYear {
		t.Errorf("expected year %q, got %q", expectedYear, year)
	}

	rawWithDate := []byte("Date: Mon, 15 Jan 2024 10:00:00 +0000\r\nSubject: Dated Email\r\n\r\nBody")
	if storeErr := msg.Store(cachedir.At(tmpDir), "msg_with_date", rawWithDate, now); storeErr != nil {
		t.Fatalf("failed to store message: %v", storeErr)
	}

	emDated, _, yearDated, monthDated, err := parseEmailDate(cachedir.At(tmpDir), "msg_with_date")
	if err != nil {
		t.Fatalf("parseEmailDate failed: %v", err)
	}
	if yearDated != "2024" || monthDated != "01" {
		t.Errorf("expected 2024/01, got %s/%s", yearDated, monthDated)
	}
	if emDated.EmailDate.Year() != 2024 {
		t.Errorf("expected parsed year 2024, got %d", emDated.EmailDate.Year())
	}
}
