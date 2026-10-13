package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
)

// --json is only useful if nothing else lands on stdout. The status stream
// carries progress, warnings and deprecation notices; if any of them leaked
// into the product stream the output would not parse.
func TestJSONOutputIsParseableWithStatusPresent(t *testing.T) {
	var out, status bytes.Buffer
	session := &Session{
		Config: &cfg_g.Config{SelectedAccount: "work"},
		Out:    &out,
		Err:    &status,
	}

	accounts := []cfg_acc.AccountConfig{
		{Name: "default", Type: "gmail"},
		{Name: "work", DisplayName: "Work", Type: "outlook", AccountType: "test"},
	}

	// Something noisy on the status stream, as a real run would have.
	session.status().Warnf("a deprecation notice")

	if err := session.emitJSON(accountListing(session, accounts)); err != nil {
		t.Fatalf("emitJSON: %v", err)
	}

	var got []accountJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("stdout did not parse as JSON: %v\n%s", err, out.String())
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(got))
	}
	if got[1].Name != "work" || !got[1].Selected {
		t.Errorf("expected work to be the selected account, got %+v", got[1])
	}
	if got[0].AccountType != "regular" {
		t.Errorf("an unset account_type should read as regular, got %q", got[0].AccountType)
	}
	if !strings.Contains(status.String(), "deprecation notice") {
		t.Error("the warning should have gone to the status stream")
	}
}

// The JSON listing must honour the same --all filtering as the text one, or the
// two disagree about what a listing is.
func TestRuleListingHonoursTheAllFlag(t *testing.T) {
	rules := []cfg_acc.Rule{
		{Sender: "a@example.com", Label: "A", Exported: false},
		{Sender: "b@example.com", Label: "B", Exported: true},
	}

	defer func(prev bool) { flagRuleListAll = prev }(flagRuleListAll)

	flagRuleListAll = false
	if got := ruleListing(rules); len(got) != 1 || got[0].Sender != "a@example.com" {
		t.Errorf("without --all, exported rules should be hidden, got %+v", got)
	}

	flagRuleListAll = true
	if got := ruleListing(rules); len(got) != 2 {
		t.Errorf("with --all, every rule should appear, got %+v", got)
	}
}

func TestWantJSONReadsTheFlag(t *testing.T) {
	defer func(prev bool) { app.FlagJSON = prev }(app.FlagJSON)

	app.FlagJSON = false
	if wantJSON() {
		t.Error("expected wantJSON to be false by default")
	}
	app.FlagJSON = true
	if !wantJSON() {
		t.Error("expected wantJSON to follow the flag")
	}
}

// scan is a data-producing command, so --json has to reach scan.Options, or the
// flag is accepted and silently ignored.
func TestScanLabelRefThreadsJSONFlag(t *testing.T) {
	defer func(prev bool) { app.FlagJSON = prev }(app.FlagJSON)

	rec := &sessionRecorder{}
	session := recordingSession(rec)

	app.FlagJSON = true
	if err := scanLabelRef(session, "%inbox"); err != nil {
		t.Fatalf("scanLabelRef: %v", err)
	}
	if !rec.opts.JSON {
		t.Error("--json should reach scan.Options.JSON")
	}
}
