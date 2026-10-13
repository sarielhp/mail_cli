package outlook

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
)

func TestOutlookClient_ExportRules_Errors(t *testing.T) {
	t.Run("GET rules failure", func(t *testing.T) {
		client := &client{
			config:  &cfg_g.Config{},
			account: cfg_acc.AccountConfig{Name: "test"},
			client: &http.Client{
				Transport: mockTransport(func(_ *http.Request) *http.Response {
					return &http.Response{
						StatusCode: http.StatusUnauthorized,
						Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":"unauthorized"}`))),
						Header:     make(http.Header),
					}
				}),
			},
		}
		err := client.ExportRules()
		if err == nil {
			t.Fatal("expected error on 401 GET, got nil")
		}
		if !strings.Contains(err.Error(), "fetching rules failed: status 401") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("DELETE existing rule failure", func(t *testing.T) {
		client := &client{
			config:  &cfg_g.Config{},
			account: cfg_acc.AccountConfig{Name: "test"},
			client: &http.Client{
				Transport: mockTransport(func(req *http.Request) *http.Response {
					if req.Method == http.MethodGet {
						jsonResp := `{"value":[{"id":"r1","displayName":"mail_cli_rule_1"}]}`
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(bytes.NewReader([]byte(jsonResp))),
							Header:     make(http.Header),
						}
					}
					return &http.Response{
						StatusCode: http.StatusInternalServerError,
						Body:       io.NopCloser(bytes.NewReader([]byte("server error"))),
						Header:     make(http.Header),
					}
				}),
			},
		}
		err := client.ExportRules()
		if err == nil {
			t.Fatal("expected error on failed DELETE, got nil")
		}
		if !strings.Contains(err.Error(), "deleting rule r1 failed: status 500") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("POST rule failure", func(t *testing.T) {
		client := &client{
			config: &cfg_g.Config{Policy: cfg_g.Policy{Rules: []cfg_acc.Rule{
				{Label: "Inbox", Sender: "spam@example.com"},
			}}},
			account: cfg_acc.AccountConfig{Name: "test"},
			client: &http.Client{
				Transport: mockTransport(func(req *http.Request) *http.Response {
					if req.Method == http.MethodGet {
						if strings.Contains(req.URL.String(), "messagerules") {
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       io.NopCloser(bytes.NewReader([]byte(`{"value":[]}`))),
								Header:     make(http.Header),
							}
						}
						// Folder lookup
						foldersResp := `{"value":[{"id":"f-inbox","displayName":"Inbox"}]}`
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(bytes.NewReader([]byte(foldersResp))),
							Header:     make(http.Header),
						}
					}
					// POST rule fails
					return &http.Response{
						StatusCode: http.StatusBadRequest,
						Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":"bad_request"}`))),
						Header:     make(http.Header),
					}
				}),
			},
		}
		err := client.ExportRules()
		if err == nil {
			t.Fatal("expected error on failed POST, got nil")
		}
		if !strings.Contains(err.Error(), "failed to export rule 1: status 400") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}
