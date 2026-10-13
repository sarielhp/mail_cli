package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/sarielhp/mail_cli/cfg_g"
)

// Defaults for an LLM request: the model used when the caller names none, the
// sampling parameters, and the system prompt the summary is asked for.
const (
	defaultSystemPrompt = "You are an expert executive assistant.\n" +
		"Analyze the following email and return a concise summary matching this format:\n\n" +
		"**Urgency**: [High / Medium / Low] | **Reply Required**: [Yes / No]\n" +
		"**Deadlines**: (List any actionable deadlines with dates/times, or \"None\")\n\n" +
		"**TL;DR**: 1-2 sentence core overview.\n" +
		"**Action Items**: Tasks with assigned owners and deadlines (or \"None\").\n" +
		"**Key Dates & Locations**: Important events, syncs, or links (or \"None\")."

	DefaultModel       = "llama3.1"
	defaultTemperature = 0.2
	defaultMaxTokens   = 400
	defaultTimeout     = 60 * time.Second

	// DefaultMaxInputChars bounds the size of the email text handed to the
	// model. Summarizing is lossy anyway, and a single message that exceeds the
	// model's context window is rejected outright, so the head of an over-long
	// message is kept and the tail dropped: that is where the sender, subject
	// and first paragraphs live, and dropping the end costs the least meaning.
	//
	// The bound is in characters, not tokens, because no tokenizer is available
	// here. At the ~4 characters-per-token ratio English tends toward, 60000
	// characters is ~15000 tokens -- comfortably inside a 32768-token input
	// window while leaving room for the system prompt and the reply.
	DefaultMaxInputChars = 60000

	// truncatedNotice marks where content was cut so the model does not treat
	// the last line it can see as the end of the message.
	truncatedNotice = "\n\n[... message truncated ...]"
)

type deadlineItem struct {
	RawText     string
	Date        string
	Description string
}

// ParsedSummary is a message summary as returned by the model: a short TLDR,
// any deadlines it found, whether a reply is needed, and the raw model text it
// was parsed from.
type ParsedSummary struct {
	Raw           string
	Urgency       string
	ReplyRequired bool
	Deadlines     []deadlineItem
	TLDR          string
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
	// ReasoningEffort is omitted when empty so a backend that does not
	// implement it never sees the field; "none" is how a hybrid model such as
	// Qwen3 is told not to emit a thinking trace through the OpenAI API.
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// NormalizeEndpoint ensures the endpoint URL points to the chat completions API path.
func NormalizeEndpoint(endpoint string) string {
	ep := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if ep == "" {
		return ""
	}
	if strings.HasSuffix(ep, "/chat/completions") {
		return ep
	}
	if strings.HasSuffix(ep, "/v1") {
		return ep + "/chat/completions"
	}
	return ep + "/v1/chat/completions"
}

// TruncateForModel caps the email text sent to the model at limit bytes, on a
// rune boundary so a multi-byte character is never split, and appends a notice
// so the model can tell the message was cut. A limit smaller than the notice
// falls back to a plain rune-boundary cut, and limit <= 0 disables truncation.
// It exists because a message larger than the model's input window is refused
// outright, which costs the whole summary for the sake of the last paragraphs.
func TruncateForModel(s string, limit int) string {
	if limit <= 0 || len(s) <= limit {
		return s
	}
	if limit <= len(truncatedNotice) {
		return truncateFront(s, limit)
	}
	return truncateFront(s, limit-len(truncatedNotice)) + truncatedNotice
}

// truncateFront returns the first n bytes of s, walking back to the nearest rune
// start on a multi-byte boundary.
func truncateFront(s string, n int) string {
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func buildChatCompletionRequest(cfg cfg_g.LLMConfig, emailContent string) chatCompletionRequest {
	model := cfg.Model
	if model == "" {
		model = DefaultModel
	}
	temp := defaultTemperature
	if cfg.Temperature != nil {
		temp = *cfg.Temperature
	}

	maxTokens := defaultMaxTokens
	if cfg.MaxTokens != nil && *cfg.MaxTokens > 0 {
		maxTokens = *cfg.MaxTokens
	}

	sysPrompt := cfg.SystemPrompt
	if strings.TrimSpace(sysPrompt) == "" {
		sysPrompt = defaultSystemPrompt
	}

	return chatCompletionRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: sysPrompt},
			{Role: "user", Content: emailContent},
		},
		Temperature:     temp,
		MaxTokens:       maxTokens,
		ReasoningEffort: strings.TrimSpace(cfg.ReasoningEffort),
	}
}

func createLLMHTTPRequest(ctx context.Context, endpoint, apiKey string, body []byte) (*http.Request, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		authVal := apiKey
		if !strings.HasPrefix(strings.ToLower(authVal), "bearer ") {
			authVal = "Bearer " + authVal
		}
		httpReq.Header.Set("Authorization", authVal)
	}
	return httpReq, nil
}

func parseChatCompletionResponse(respBody []byte) (string, error) {
	var completionResp chatCompletionResponse
	if err := json.Unmarshal(respBody, &completionResp); err != nil {
		return "", fmt.Errorf("failed to parse LLM JSON response: %w", err)
	}

	if completionResp.Error != nil && completionResp.Error.Message != "" {
		return "", fmt.Errorf("LLM error: %s", completionResp.Error.Message)
	}

	if len(completionResp.Choices) == 0 {
		return "", fmt.Errorf("LLM returned no response choices")
	}

	return strings.TrimSpace(completionResp.Choices[0].Message.Content), nil
}

// SummarizeEmail sends an email text to the configured LLM endpoint and returns the executive summary.
func SummarizeEmail(ctx context.Context, cfg cfg_g.LLMConfig, emailContent string) (string, error) {
	endpoint := NormalizeEndpoint(cfg.Endpoint)
	if endpoint == "" {
		return "", fmt.Errorf("LLM endpoint is not configured")
	}

	reqPayload := buildChatCompletionRequest(cfg, TruncateForModel(emailContent, DefaultMaxInputChars))
	bodyBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request payload: %w", err)
	}

	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()
	}

	httpReq, err := createLLMHTTPRequest(ctx, endpoint, cfg.APIKey, bodyBytes)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: defaultTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("LLM request failed: %w", err)
	}
	defer resp.Body.Close()

	const maxErrorBodyBytes = 4096
	const maxLLMResponseBytes = 10 * 1024 * 1024 // 10MB limit

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		return "", fmt.Errorf("LLM endpoint returned HTTP status %d: %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
	}

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxLLMResponseBytes))
	if err != nil {
		return "", fmt.Errorf("failed to read LLM response body: %w", err)
	}

	return parseChatCompletionResponse(respBody)
}

var (
	reUrgency  = regexp.MustCompile(`(?i)\*\*Urgency\*\*:\s*(High|Medium|Low)`)
	reReplyReq = regexp.MustCompile(`(?i)\*\*Reply Required\*\*:\s*(Yes|No)`)
	reTLDR     = regexp.MustCompile(`(?i)\*\*TL;DR\*\*:\s*(.*)`)
)

// ParseSummary parses structured attributes (urgency, reply required, deadlines) from a summary text.
func ParseSummary(raw string) ParsedSummary {
	p := ParsedSummary{
		Raw:           raw,
		Urgency:       "Low",
		ReplyRequired: false,
	}

	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")

	inDeadlines := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Urgency
		if match := reUrgency.FindStringSubmatch(trimmed); len(match) > 1 {
			p.Urgency = cases.Title(language.English).String(strings.ToLower(match[1]))
		}

		// Reply Required
		if match := reReplyReq.FindStringSubmatch(trimmed); len(match) > 1 {
			p.ReplyRequired = strings.EqualFold(match[1], "yes")
		}

		// TLDR
		if match := reTLDR.FindStringSubmatch(trimmed); len(match) > 1 {
			p.TLDR = strings.TrimSpace(match[1])
		}

		// Deadlines section
		if strings.HasPrefix(strings.ToLower(trimmed), "**deadlines**:") {
			inDeadlines = true
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, trimmed[:len("**deadlines**:")]))
			if rest != "" && !strings.EqualFold(rest, "none") && !strings.EqualFold(rest, "none.") {
				p.Deadlines = append(p.Deadlines, deadlineItem{
					RawText:     rest,
					Description: rest,
				})
			}
			continue
		}

		if inDeadlines {
			if strings.HasPrefix(trimmed, "**") && strings.Contains(trimmed, ":") {
				inDeadlines = false
			} else if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "• ") {
				itemText := strings.TrimLeft(trimmed, "-*• \t")
				if itemText != "" && !strings.EqualFold(itemText, "none") && !strings.EqualFold(itemText, "none.") {
					p.Deadlines = append(p.Deadlines, deadlineItem{
						RawText:     itemText,
						Description: itemText,
					})
				}
			}
		}
	}

	return p
}
