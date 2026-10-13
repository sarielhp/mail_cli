package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/llm"

	"github.com/sarielhp/clihelp"
)

// llmTestSample is deliberately trivial: the point is to exercise the endpoint,
// not the summarizer, so a message whose summary cannot matter keeps a
// failure meaningful.
const llmTestSample = "Subject: Leftover cake in the break room\n\n" +
	"Hi team, someone left a cake in the break room. First come, first served. " +
	"No reply needed.\n"

// llmTestMarkers are the headings ParseSummary keys on. A reply without them
// cannot be parsed however fluent it is.
var llmTestMarkers = []string{"**Urgency**", "**TL;DR**"}

// LLMCmd groups the commands that exercise the configured LLM endpoint.
func LLMCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "llm",
		Group:       "Setup",
		Description: "Test the configured LLM endpoint.",
		UsageLine:   "mail_cli config llm <subcommand>",
		Subcommands: []clihelp.Command{
			llmTestCmd(session),
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli config llm test", Description: "Send a harmless sample and check the answer"},
		},
	}
}

// llmTestCmd sends one sample message through the same path a scan uses, so a
// broken endpoint, a missing model or a leaking thinking trace shows up here
// rather than in the middle of a summary the user is reading.
func llmTestCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "test",
		Aliases:     []string{"check"},
		Title:       "config llm test",
		Description: "Send a sample email to the configured LLM.",
		LongDescription: "Send a harmless sample message to the configured chat endpoint " +
			"and report whether the reply carries the urgency and TL;DR " +
			"fields the summarizer parses. Use it to confirm an endpoint, model or " +
			"reasoning-effort change before scanning real mail.",
		UsageLine: "mail_cli config llm test",
		Args:      clihelp.NoArgs,
		Run: func(ctx *clihelp.Context) error {
			return runLLMTest(session, ctxStdout(ctx))
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli config llm test", Description: "Check the endpoint, model and answer format"},
		},
	}
}

func runLLMTest(session *Session, w io.Writer) error {
	cfg := session.Config.LLM
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return fmt.Errorf("LLM endpoint is not configured; set one with %q", "mail_cli config set llm_endpoint <url>")
	}

	printLLMTestHeader(w, cfg)
	summary, err := llm.SummarizeEmail(context.Background(), cfg, llmTestSample)
	if err != nil {
		return fmt.Errorf("LLM test failed: %w", err)
	}

	fmt.Fprintf(w, "%s\n\n", strings.TrimSpace(summary))
	return reportLLMTestResult(w, summary)
}

func printLLMTestHeader(w io.Writer, cfg cfg_g.LLMConfig) {
	model := cfg.Model
	if model == "" {
		model = llm.DefaultModel
	}
	effort := cfg.ReasoningEffort
	if effort == "" {
		effort = "(server default)"
	}
	fmt.Fprintf(w, "Testing LLM endpoint %s\n", cfg.Endpoint)
	fmt.Fprintf(w, "  Model:            %s\n", model)
	fmt.Fprintf(w, "  Reasoning effort: %s\n", effort)
	fmt.Fprintf(w, "Sending a harmless sample message...\n\n")
}

func reportLLMTestResult(w io.Writer, summary string) error {
	missing := missingLLMMarkers(summary)
	if len(missing) > 0 {
		fmt.Fprintf(w, "%s The reply is missing fields the summarizer needs: %s\n",
			app.PrefixError, strings.Join(missing, ", "))
		return fmt.Errorf("LLM reply did not match the expected summary format")
	}

	parsed := llm.ParseSummary(summary)
	fmt.Fprintf(w, "%s LLM replied with a parsable summary.\n", app.PrefixSuccess)
	fmt.Fprintf(w, "    Urgency:        %s\n", parsed.Urgency)
	fmt.Fprintf(w, "    Reply required: %t\n", parsed.ReplyRequired)
	if parsed.TLDR != "" {
		fmt.Fprintf(w, "    TL;DR:          %s\n", parsed.TLDR)
	}
	return nil
}

func missingLLMMarkers(summary string) []string {
	var missing []string
	for _, marker := range llmTestMarkers {
		if !strings.Contains(summary, marker) {
			missing = append(missing, marker)
		}
	}
	return missing
}
