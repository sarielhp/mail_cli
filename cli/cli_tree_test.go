package cli

import (
	"bytes"
	"math/rand"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/last"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/scan"

	"github.com/sarielhp/clihelp"
)

func generateRandomCmdString(rng *rand.Rand, length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rng.Intn(len(charset))]
	}
	return "rand_" + string(b)
}

func getAllCommandNames(cliApp *clihelp.App) map[string]bool {
	names := make(map[string]bool)
	_ = cliApp.Walk(func(_ []string, cmd *clihelp.Command) error {
		names[strings.ToLower(cmd.Name)] = true
		for _, alias := range cmd.Aliases {
			names[strings.ToLower(alias)] = true
		}
		return nil
	})
	return names
}

func collectAllCommandPaths(cliApp *clihelp.App) [][]string {
	var paths [][]string
	_ = cliApp.Walk(func(path []string, _ *clihelp.Command) error {
		paths = append(paths, path)
		return nil
	})
	return paths
}

func TestRandomRootCommandsReject(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	session := &Session{
		Config: &cfg_g.Config{},
	}
	cliApp := InitCLI(session)
	knownCmds := getAllCommandNames(cliApp)

	for i := 0; i < 25; i++ {
		randStr := generateRandomCmdString(rng, 8+rng.Intn(8))
		if knownCmds[randStr] {
			continue
		}

		err := cliApp.Execute([]string{randStr})
		if err == nil {
			t.Errorf("expected error for random root command %q, got nil", randStr)
			continue
		}
		if !strings.Contains(err.Error(), "unknown command") && !strings.Contains(err.Error(), randStr) {
			t.Errorf("error for random command %q did not mention unknown command: %v", randStr, err)
		}
	}
}

func TestRandomSubcommandPathsReject(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	session := &Session{
		Config: &cfg_g.Config{},
	}
	cliApp := InitCLI(session)
	paths := collectAllCommandPaths(cliApp)

	for _, path := range paths {
		cmd := cliApp.LookupCommand(path...)
		if cmd == nil || len(cmd.Subcommands) == 0 {
			continue
		}
		randStr := generateRandomCmdString(rng, 10)
		invalidPath := append(append([]string{}, path...), randStr)

		err := cliApp.Execute(invalidPath)
		if err == nil {
			t.Errorf("expected error when executing invalid subcommand path %v, got nil", invalidPath)
		}
	}
}

func TestCommandPrefixesYieldUsage(t *testing.T) {
	session := &Session{
		Config: &cfg_g.Config{},
	}
	cliApp := InitCLI(session)
	paths := collectAllCommandPaths(cliApp)

	for _, path := range paths {
		var buf bytes.Buffer
		opts := clihelp.Options{
			Writer: &buf,
			Width:  80,
		}

		rendered := cliApp.RenderCommand(opts, path...)
		if !rendered {
			t.Errorf("failed to render usage for command path %v", path)
			continue
		}

		output := buf.String()
		if len(strings.TrimSpace(output)) == 0 {
			t.Errorf("rendered usage for path %v was empty", path)
			continue
		}

		cmdLeaf := path[len(path)-1]
		if !strings.Contains(output, cmdLeaf) {
			t.Errorf("rendered usage for path %v does not contain command name %q:\n%s", path, cmdLeaf, output)
		}
	}
}

// clihelp shows a command's own help when it is invoked with nothing and needs
// something. Two details differ from the wrapper this replaced, and both are
// deliberate on the library's side: the help goes to stderr, because it is a
// response to an error and stdout belongs to whatever is reading the command's
// output; and the error is still returned, so "mail_cli label download" with an empty
// shell variable fails a script instead of reporting success.
func TestDownloadShowsUsageWithoutArgs(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	session := &Session{Config: &cfg_g.Config{}}
	cliApp := InitCLI(session)
	cliApp.Stdout = &outBuf
	cliApp.Stderr = &errBuf

	err := cliApp.Execute([]string{"label", "download"})
	if err == nil {
		t.Fatal("expected the missing-argument error; a script must not read this as success")
	}

	out := errBuf.String()
	if !strings.Contains(out, "Usage:  mail_cli label download") {
		t.Errorf("expected usage output, got:\n%s", out)
	}
	if !strings.Contains(out, "Examples:") {
		t.Errorf("expected Examples in usage output, got:\n%s", out)
	}
	if !strings.Contains(out, "<label_or_prefix>") {
		t.Errorf("expected parameters in usage output, got:\n%s", out)
	}
}

// Each of these commands requires arguments, so invoking it bare answers with
// its own help — naming the parameters the caller is missing — on stderr, and
// still fails.
func TestSubcommandsShowUsageWithoutRequiredArgs(t *testing.T) {
	session := &Session{Config: &cfg_g.Config{}}
	cliApp := InitCLI(session)

	testCases := []struct {
		path     []string
		expected []string
	}{
		{[]string{"label", "upload"}, []string{"Usage:  mail_cli label upload", "<label>", "<file_name>"}},
		{[]string{"migrate"}, []string{"Usage:  mail_cli migrate", "[user@]host"}},
		{[]string{"label", "sort"}, []string{"Usage:  mail_cli label sort", "<folder>"}},
		{[]string{"last"}, []string{"Usage:  mail_cli last", "<N>"}},
		{[]string{"rule", "add"}, []string{"Usage:  mail_cli rule add", "<email|message_id>"}},
		{[]string{"config", "set"}, []string{"Usage:  mail_cli config set", "<key>", "<value>"}},
	}

	for _, tc := range testCases {
		var outBuf, errBuf bytes.Buffer
		cliApp.Stdout = &outBuf
		cliApp.Stderr = &errBuf

		err := cliApp.Execute(tc.path)
		if err == nil {
			t.Errorf("expected a missing-argument error for %v", tc.path)
		}
		out := errBuf.String()
		for _, exp := range tc.expected {
			if !strings.Contains(out, exp) {
				t.Errorf("command path %v output missing %q:\n%s", tc.path, exp, out)
			}
		}
		if outBuf.Len() != 0 {
			t.Errorf("command path %v wrote to stdout on failure: %q", tc.path, outBuf.String())
		}
	}
}

func TestAllSubcommandsShowUsageOrTriggerDefaultAction(t *testing.T) {
	tempDir := t.TempDir()
	session := &Session{
		Config: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: filepath.Join(tempDir, "cache")}},
		Deps: Deps{
			Clients: stubClients{
				PreCheckFn: func(_ *cfg_g.Config) error { return nil },
			},
			Mail: stubMail{
				ScanFn: func(_ scan.Options, _ *cfg_g.Config, _ string) (int, error) { return 0, nil },
				LastFn: func(_ last.Options, _ *cfg_g.Config, _ int) error { return nil },
			},
			Spam: stubSpam{
				ResetLearningFn: func(_ *cfg_g.Config) error { return nil },
			},
			Config: stubConfig{
				ShowFn:     func(_ *cfg_g.Config) error { return nil },
				ValidateFn: func(_ *cfg_g.Config) error { return nil },
			},
			Calendar: stubCalendar{
				WeekFn:   func(_ *cfg_g.Config, _ ...int) error { return nil },
				AddAllFn: func(_ *cfg_g.Config, _ mailclient.MailClient) error { return nil },
			},
			TUI: stubTUI{
				RunFn: func(_ *cfg_g.Config, _ string, _ ...string) error { return nil },
			},
		},
	}
	cliApp := InitCLI(session)
	paths := collectAllCommandPaths(cliApp)

	for _, path := range paths {
		var outBuf, errBuf bytes.Buffer
		cliApp.Stdout = &outBuf
		cliApp.Stderr = &errBuf

		_ = cliApp.Execute(path)
		out := outBuf.String()
		errStr := errBuf.String()
		combined := out + "\n" + errStr

		if strings.Contains(combined, "accepts ") && strings.Contains(combined, "received 0") {
			t.Errorf("command path %v failed with generic argument error instead of showing usage: %s", path, combined)
		}
		if strings.Contains(combined, "requires at least") && strings.Contains(combined, "only received 0") {
			t.Errorf("command path %v failed with generic argument error instead of showing usage: %s", path, combined)
		}
		if strings.Contains(combined, "exact") && strings.Contains(combined, "arg(s)") && strings.Contains(combined, "received 0") {
			t.Errorf("command path %v failed with generic argument error instead of showing usage: %s", path, combined)
		}
	}
}

func TestCLIAudit(t *testing.T) {
	session := &Session{Config: &cfg_g.Config{}}
	cliApp := InitCLI(session)

	if err := clihelp.Audit(cliApp); err != nil {
		t.Fatalf("clihelp.Audit failed: %v", err)
	}
}
