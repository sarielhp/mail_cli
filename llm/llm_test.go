package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/sarielhp/mail_cli/cfg_g"
)

func TestNormalizeEndpoint(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "", want: ""},
		{input: "http://localhost:4000", want: "http://localhost:4000/v1/chat/completions"},
		{input: "http://localhost:4000/", want: "http://localhost:4000/v1/chat/completions"},
		{input: "http://localhost:4000/v1", want: "http://localhost:4000/v1/chat/completions"},
		{input: "http://localhost:4000/v1/", want: "http://localhost:4000/v1/chat/completions"},
		{input: "http://localhost:4000/v1/chat/completions", want: "http://localhost:4000/v1/chat/completions"},
		{input: "http://custom-host:4000/chat/completions", want: "http://custom-host:4000/chat/completions"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeEndpoint(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeEndpoint(%q) = %q; want %q", tt.input, got, tt.want)
			}
			// A normalized endpoint is fed back in every time a request is made,
			// so it has to be a fixed point.
			if again := NormalizeEndpoint(got); again != got {
				t.Errorf("NormalizeEndpoint is not idempotent for %q: %q then %q", tt.input, got, again)
			}
		})
	}
}

func TestSummarizeEmail_Success(t *testing.T) {
	expectedSummary := "**TL;DR**: The team agreed on project milestones.\n**Action Items**: None\n**Key Dates & Deadlines**: Friday 5 PM"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test-key" {
			t.Errorf("expected Authorization header 'Bearer sk-test-key', got %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		var req chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		if req.Model != "llama3.1" {
			t.Errorf("expected model llama3.1, got %s", req.Model)
		}
		if req.Temperature != 0.2 {
			t.Errorf("expected temperature 0.2, got %f", req.Temperature)
		}
		if req.MaxTokens != 400 {
			t.Errorf("expected max_tokens 400, got %d", req.MaxTokens)
		}
		if len(req.Messages) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(req.Messages))
		}
		if req.ReasoningEffort != "" {
			t.Errorf("expected reasoning_effort to be empty when unset, got %q", req.ReasoningEffort)
		}

		resp := chatCompletionResponse{
			Choices: []struct {
				Message struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"message"`
			}{
				{
					Message: struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					}{
						Role:    "assistant",
						Content: expectedSummary,
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := cfg_g.LLMConfig{
		Endpoint: server.URL,
		APIKey:   "sk-test-key",
	}

	summary, err := SummarizeEmail(context.Background(), cfg, "Hello team, let's meet on Friday at 5 PM.")
	if err != nil {
		t.Fatalf("SummarizeEmail failed: %v", err)
	}

	if summary != expectedSummary {
		t.Errorf("SummarizeEmail() = %q; want %q", summary, expectedSummary)
	}
}

func TestSummarizeEmail_Errors(t *testing.T) {
	// 1. Missing endpoint
	_, err := SummarizeEmail(context.Background(), cfg_g.LLMConfig{}, "test")
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected unconfigured endpoint error, got %v", err)
	}

	// 2. HTTP Server Error (500)
	server500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server500.Close()

	_, err = SummarizeEmail(context.Background(), cfg_g.LLMConfig{Endpoint: server500.URL}, "test")
	if err == nil || !strings.Contains(err.Error(), "HTTP status 500") {
		t.Errorf("expected 500 error, got %v", err)
	}

	// 3. Invalid JSON
	serverBadJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not-json"))
	}))
	defer serverBadJSON.Close()

	_, err = SummarizeEmail(context.Background(), cfg_g.LLMConfig{Endpoint: serverBadJSON.URL}, "test")
	if err == nil || !strings.Contains(err.Error(), "failed to parse LLM JSON response") {
		t.Errorf("expected JSON parse error, got %v", err)
	}

	// 4. Empty choices
	serverEmptyChoices := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices": []}`))
	}))
	defer serverEmptyChoices.Close()

	_, err = SummarizeEmail(context.Background(), cfg_g.LLMConfig{Endpoint: serverEmptyChoices.URL}, "test")
	if err == nil || !strings.Contains(err.Error(), "no response choices") {
		t.Errorf("expected no choices error, got %v", err)
	}
}

// A configured reasoning effort must reach the wire, and its absence must not:
// the field is how a hybrid model is told to skip its thinking trace, and an
// empty value sent to a backend that does not know it would be a rejection.
func TestSummarizeEmail_SendsReasoningEffort(t *testing.T) {
	tests := []struct {
		name   string
		effort string
		want   string
	}{
		{"none disables thinking", "none", `"reasoning_effort":"none"`},
		{"medium enables thinking", "medium", `"reasoning_effort":"medium"`},
		{"empty omits the field", "", `"reasoning_effort"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rawBody string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				rawBody = string(body)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
			}))
			defer server.Close()

			cfg := cfg_g.LLMConfig{Endpoint: server.URL, ReasoningEffort: tt.effort}
			if _, err := SummarizeEmail(context.Background(), cfg, "test"); err != nil {
				t.Fatalf("SummarizeEmail failed: %v", err)
			}

			present := strings.Contains(rawBody, tt.want)
			if tt.effort == "" && present {
				t.Errorf("empty effort should omit reasoning_effort, body was %s", rawBody)
			}
			if tt.effort != "" && !present {
				t.Errorf("expected %s in body, got %s", tt.want, rawBody)
			}
		})
	}
}

func TestParseSummary(t *testing.T) {
	raw := `**Classification**: Spam — Suspicious cryptocurrency giveaway
**Urgency**: High | **Reply Required**: No
**Deadlines**:
• 2026-08-25 17:00: Claim your bonus tokens

**TL;DR**: The sender is promising free crypto in exchange for wallet credentials.
**Action Items**:
• Do not click any links.
**Key Dates & Locations**:
• None`

	parsed := ParseSummary(raw)
	if parsed.Urgency != "High" {
		t.Errorf("expected Urgency='High', got %s", parsed.Urgency)
	}
	if parsed.ReplyRequired {
		t.Errorf("expected ReplyRequired=false, got true")
	}
	if len(parsed.Deadlines) != 1 {
		t.Fatalf("expected 1 deadline, got %d", len(parsed.Deadlines))
	}
	if !strings.Contains(parsed.Deadlines[0].Description, "Claim your bonus tokens") {
		t.Errorf("unexpected deadline description: %s", parsed.Deadlines[0].Description)
	}
	if !strings.Contains(parsed.TLDR, "promising free crypto") {
		t.Errorf("unexpected TLDR: %s", parsed.TLDR)
	}
}

func TestSummarizeEmail_ErrorBodyTruncation(t *testing.T) {
	hugeError := strings.Repeat("E", 100*1024)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(hugeError))
	}))
	defer server.Close()

	_, err := SummarizeEmail(context.Background(), cfg_g.LLMConfig{Endpoint: server.URL}, "test")
	if err == nil {
		t.Fatalf("expected error from 500 status")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "HTTP status 500") {
		t.Errorf("expected HTTP status 500 in error, got: %s", errStr)
	}
	if len(errStr) > 5000 {
		t.Errorf("expected error message to be bounded under 5000 chars, got len=%d", len(errStr))
	}
}

func TestTruncateForModel(t *testing.T) {
	body := strings.Repeat("a", 100)

	tests := []struct {
		name   string
		input  string
		limit  int
		assert func(t *testing.T, got string)
	}{
		{
			name:  "short input is untouched",
			input: "hello",
			limit: 1000,
			assert: func(t *testing.T, got string) {
				if got != "hello" {
					t.Errorf("expected the input unchanged, got %q", got)
				}
			},
		},
		{
			name:  "limit of zero disables truncation",
			input: body,
			limit: 0,
			assert: func(t *testing.T, got string) {
				if got != body {
					t.Errorf("expected limit 0 to leave the message alone")
				}
			},
		},
		{
			name:  "over-long input is cut and annotated",
			input: body,
			limit: 40,
			assert: func(t *testing.T, got string) {
				if !strings.HasSuffix(got, truncatedNotice) {
					t.Fatalf("expected the truncation notice, got %q", got)
				}
				if !strings.HasPrefix(got, strings.Repeat("a", 40-len(truncatedNotice))) {
					t.Errorf("expected the head of the message to survive, got %q", got)
				}
			},
		},
		{
			name:  "a limit inside the notice still cuts on a rune boundary",
			input: body,
			limit: 10,
			assert: func(t *testing.T, got string) {
				if got != "aaaaaaaaaa" {
					t.Errorf("expected a plain 10-byte cut, got %q", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.assert(t, TruncateForModel(tt.input, tt.limit))
		})
	}
}

// A multi-byte character straddling the cut must not be split into invalid
// UTF-8, which json.Marshal would then mangle.
func TestTruncateForModelKeepsRunesWhole(t *testing.T) {
	// Each Hebrew letter is two bytes, so a 21-byte limit lands mid-rune.
	input := strings.Repeat("\u05d0", 50)
	got := TruncateForModel(input, 21)

	if !utf8.ValidString(got) {
		t.Fatalf("truncation produced invalid UTF-8: %q", got)
	}
	if len(got) > 21 {
		t.Errorf("expected at most 21 bytes, got %d", len(got))
	}
}

// The guard has to engage through SummarizeEmail, not only when called directly.
func TestSummarizeEmailTruncatesOversizedInput(t *testing.T) {
	var received string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		received = req.Messages[1].Content
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()

	oversized := strings.Repeat("x", DefaultMaxInputChars+5000)
	if _, err := SummarizeEmail(context.Background(), cfg_g.LLMConfig{Endpoint: server.URL}, oversized); err != nil {
		t.Fatalf("SummarizeEmail failed: %v", err)
	}

	if len(received) > DefaultMaxInputChars {
		t.Errorf("expected the payload capped at %d bytes, got %d", DefaultMaxInputChars, len(received))
	}
	if !strings.HasSuffix(received, truncatedNotice) {
		t.Errorf("expected the truncated payload to carry the notice")
	}
}
