package email

import (
	"html"
	"strings"
	"testing"
)

func TestHTMLURLsSurviveRendering(t *testing.T) {
	target := "https://example.com/" + strings.Repeat("long-path/", 20) + "?a=1&b=2"
	source := "<p>MURL0X before " + html.EscapeString(target) + " after.</p>"
	got := renderHTMLWithFallback(source)
	if !strings.Contains(got, target) {
		t.Errorf("URL was split or escaped: %q", got)
	}
	if !strings.Contains(got, "MURL0X") || !strings.Contains(got, "before") || !strings.Contains(got, "after.") {
		t.Errorf("surrounding text changed: %q", got)
	}
}
