package runner

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cli"
)

func TestExtractPipePayload(t *testing.T) {
	raw := []byte("Subject: Test Subject\r\nFrom: user@example.com\r\n\r\nHello, World!\r\n")

	// Raw payload
	gotRaw, err := extractPipePayload(raw, cli.PipeOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(gotRaw, raw) {
		t.Errorf("expected raw bytes to match")
	}

	// Headers only
	gotHeaders, err := extractPipePayload(raw, cli.PipeOptions{HeadersOnly: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedHeaders := "Subject: Test Subject\r\nFrom: user@example.com\r\n\r\n"
	if string(gotHeaders) != expectedHeaders {
		t.Errorf("expected headers %q, got %q", expectedHeaders, string(gotHeaders))
	}

	// Body only
	gotBody, err := extractPipePayload(raw, cli.PipeOptions{BodyOnly: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(gotBody), "Hello, World!") {
		t.Errorf("expected body to contain 'Hello, World!', got %q", string(gotBody))
	}
}

func TestBuildPipeCmd(t *testing.T) {
	// Simple command
	cmd, err := buildPipeCmd([]string{"cat"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Path == "" || len(cmd.Args) != 1 || cmd.Args[0] != "cat" {
		t.Errorf("unexpected cmd: %+v", cmd.Args)
	}

	// Command with arguments
	cmd, err = buildPipeCmd([]string{"git", "am", "--abort"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmd.Args) != 3 || cmd.Args[1] != "am" {
		t.Errorf("unexpected cmd args: %+v", cmd.Args)
	}

	// Shell command pipeline
	cmd, err = buildPipeCmd([]string{"grep Subject | wc -l"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmd.Args) != 3 || cmd.Args[0] != "sh" || cmd.Args[1] != "-c" {
		t.Errorf("expected shell invocation for pipeline, got %+v", cmd.Args)
	}
}
