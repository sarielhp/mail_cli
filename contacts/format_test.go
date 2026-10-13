package contacts

import (
	"bytes"
	"strings"
	"testing"
)

func TestFormatTable(t *testing.T) {
	contacts := []Contact{
		{
			Name:      "Dave Rivers",
			Emails:    []string{"rivers@cs.example.edu"},
			Nicknames: []string{"dave", "rivers"},
			Groups:    []string{"algo"},
			Source:    "mutt",
		},
	}

	var buf bytes.Buffer
	if err := FormatTable(&buf, contacts); err != nil {
		t.Fatalf("FormatTable failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Dave Rivers") || !strings.Contains(out, "rivers@cs.example.edu") {
		t.Errorf("expected Dave Rivers in output, got:\n%s", out)
	}
	if !strings.Contains(out, "algo") || !strings.Contains(out, "mutt") {
		t.Errorf("expected algo and mutt in output, got:\n%s", out)
	}
}

func TestFormatTable_Empty(t *testing.T) {
	var buf bytes.Buffer
	if err := FormatTable(&buf, nil); err != nil {
		t.Fatalf("FormatTable failed: %v", err)
	}
	if !strings.Contains(buf.String(), "No contacts found.") {
		t.Errorf("expected 'No contacts found.', got %q", buf.String())
	}
}
