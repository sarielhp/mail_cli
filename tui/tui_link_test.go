package tui

import (
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
	"strings"
	"testing"
)

func TestDetailLongLinkFromRawMessage(t *testing.T) {
	t.Setenv("TERM", "xterm-kitty")
	target := "https://example.com/path/" + strings.Repeat("long-path/", 30) + "?token=original"
	for _, contentType := range []string{"text/plain", "text/html"} {
		t.Run(contentType, func(t *testing.T) {
			body := "Please visit " + target + " for details."
			if contentType == "text/html" {
				body = "<p>Please visit <a href=\"" + target + "\">" + target + "</a> for details.</p>"
			}
			m := newDetailModel(t, 80, 40, "INBOX", nil)
			m.client.(*mailclient.MockMailClient).RawMessages = map[email.ID][]byte{
				"link": []byte("From: sender@example.com\r\nSubject: Link\r\nContent-Type: " + contentType + "; charset=utf-8\r\n\r\n" + body),
			}
			loaded := m.loadDetailCmd("link")().(detailMsg)
			m.handleDetailLoaded(loaded)
			out := m.View()
			if !strings.Contains(out, "\x1b]8;;"+target+"\x1b\\") {
				t.Errorf("full hyperlink target lost: %q", out)
			}
			visible := email.StripEscapeSequences(out)
			if strings.Contains(visible, target) || !strings.Contains(visible, "...") {
				t.Errorf("link was not shortened: %q", visible)
			}
			visible = strings.Join(strings.Fields(visible), " ")
			if !strings.Contains(visible, "Please visit") || !strings.Contains(visible, "for details.") || strings.Contains(visible, "...for") {
				t.Errorf("link context lost: %q", visible)
			}
		})
	}
}
