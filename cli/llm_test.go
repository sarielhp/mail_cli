package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_g"
)

const validSummaryReply = `{"choices":[{"message":{"role":"assistant","content":"` +
	`**Urgency**: Low | **Reply Required**: No\n` +
	`**TL;DR**: Cake in the break room."}}]}`

func llmTestSession(endpoint string) *Session {
	return &Session{Config: &cfg_g.Config{LLM: cfg_g.LLMConfig{Endpoint: endpoint}}}
}

func TestLLMTestReportsParsableSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(validSummaryReply))
	}))
	defer server.Close()

	var out bytes.Buffer
	app := InitCLI(llmTestSession(server.URL))
	app.Stdout = &out

	if err := app.Execute([]string{"config", "llm", "test"}); err != nil {
		t.Fatalf("expected llm test to succeed, got: %v", err)
	}
	if !strings.Contains(out.String(), "parsable summary") {
		t.Errorf("expected a parsable-summary verdict, got:\n%s", out.String())
	}
}

func TestLLMTestRejectsUnparsableReply(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hello there"}}]}`))
	}))
	defer server.Close()

	err := InitCLI(llmTestSession(server.URL)).Execute([]string{"config", "llm", "test"})
	if err == nil {
		t.Fatal("expected a format error for a reply without the summary headings")
	}
	if !strings.Contains(err.Error(), "did not match") {
		t.Errorf("expected a format error, got: %v", err)
	}
}

func TestLLMTestRequiresEndpoint(t *testing.T) {
	err := InitCLI(&Session{Config: &cfg_g.Config{}}).Execute([]string{"config", "llm", "test"})
	if err == nil {
		t.Fatal("expected an unconfigured-endpoint error")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected an unconfigured-endpoint error, got: %v", err)
	}
}
