package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/label"
)

func TestFormatLabelTree(t *testing.T) {
	items := []label.Item{
		{Name: "Inbox", FullName: "Inbox", MessagesTotal: 10, MessagesUnread: 2, IsLabel: true},
		{Name: "Work", FullName: "Work", MessagesTotal: 5, MessagesUnread: 0, IsLabel: true},
		{Name: "Projects", FullName: "Work/Projects", MessagesTotal: 3, MessagesUnread: 1, IsLabel: true},
	}

	var buf bytes.Buffer
	FormatLabelTree(&buf, "TEST LABELS", items, false)
	out := buf.String()

	if !strings.Contains(out, "TEST LABELS") {
		t.Errorf("expected header in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Inbox") {
		t.Errorf("expected Inbox in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Work") {
		t.Errorf("expected Work in output, got:\n%s", out)
	}
}

func TestFormatLabelTree_Empty(t *testing.T) {
	var buf bytes.Buffer
	FormatLabelTree(&buf, "EMPTY", nil, false)
	out := buf.String()

	if !strings.Contains(out, "No labels/folders found") {
		t.Errorf("expected empty message, got:\n%s", out)
	}
}
