package outlook

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/report"
	"github.com/sarielhp/mail_cli/train"
)

func TestOutlookClient_LearnSpam_CorruptedLedger(t *testing.T) {
	tempDir := t.TempDir()
	corruptedPath := train.SpamUIDPath(cachedir.At(tempDir))
	if err := os.WriteFile(corruptedPath, []byte(`{invalid-json`), 0600); err != nil {
		t.Fatal(err)
	}

	client := &client{
		config:  &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: tempDir}, AccountCache: cachedir.At(tempDir)},
		account: cfg_acc.AccountConfig{Name: "test", SpamFolder: "Junk Email"},
		rep:     report.Discard(),
		client: &http.Client{
			Transport: mockTransport(func(req *http.Request) *http.Response {
				urlStr := req.URL.String()
				if strings.Contains(urlStr, "/mailFolders") && !strings.Contains(urlStr, "/messages") {
					body := `{"value":[{"id":"folder-junk","displayName":"Junk Email"}]}`
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(body)),
						Header:     make(http.Header),
					}
				}
				if strings.Contains(urlStr, "/messages?") {
					body := `{"value":[{"id":"msg-1","isRead":true,"receivedDateTime":"2026-01-01T00:00:00Z"}]}`
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(body)),
						Header:     make(http.Header),
					}
				}
				if strings.Contains(urlStr, "/messages/msg-1/$value") {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader("Subject: spam\r\n\r\nspam body")),
						Header:     make(http.Header),
					}
				}
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader("not found")),
					Header:     make(http.Header),
				}
			}),
		},
	}

	err := client.LearnSpam()
	if err == nil {
		t.Fatal("expected error on corrupted ledger, got nil")
	}
	if !strings.Contains(err.Error(), "delete the file to start the trained-spam record over") {
		t.Errorf("expected remediation hint in error, got: %v", err)
	}
}
