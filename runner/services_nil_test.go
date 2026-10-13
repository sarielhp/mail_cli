package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
)

// A service built as a bare struct literal has a nil Rep. That is easy to do,
// it compiles, and it used to pass nil into code that dereferenced it -- `config
// show` panicked in exactly this way. Every service must degrade to silence
// instead.
func TestZeroValuedServicesDoNotPanic(t *testing.T) {
	dir := t.TempDir()
	config := &cfg_g.Config{
		Paths:    cfg_g.Paths{CacheRoot: dir, ConfigDir: dir},
		Accounts: []cfg_acc.AccountConfig{{Name: "acc", Type: "gmail"}},
	}

	calls := map[string]func() error{
		"ConfigService.Show":     func() error { return ConfigService{}.Show(config) },
		"ConfigService.Validate": func() error { return ConfigService{}.Validate(config) },
		"ConfigService.Reset":    func() error { return ConfigService{}.Reset(config, "limit") },
		"SpamService.Reset":      func() error { return SpamService{}.ResetLearning(config) },
		"ClientService.PreCheck": func() error { return ClientService{}.PreCheck(config) },
		"AccountService.Token": func() error {
			_, err := AccountService{}.TokenPath(dir, cfg_acc.AccountConfig{Name: "a", Type: "gmail"})
			return err
		},
	}

	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("%s panicked with a nil reporter: %v", name, r)
				}
			}()
			_ = call()
		})
	}
}

// The colour migration once emitted a literal backslash-n instead of a newline
// escape, so `config show` printed "CONFIGURATION\n===" on one line. Rendering
// the config to a buffer is the cheapest way to notice that again.
func TestConfigShowEmitsRealNewlines(t *testing.T) {
	dir := t.TempDir()
	config := &cfg_g.Config{
		Paths:    cfg_g.Paths{CacheRoot: dir, ConfigDir: dir},
		Accounts: []cfg_acc.AccountConfig{{Name: "acc", Type: "gmail"}},
	}

	if err := os.WriteFile(filepath.Join(dir, "config.json"),
		[]byte(`{"download_dir":"`+dir+`","accounts":[{"name":"acc","type":"gmail"}]}`), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var sb strings.Builder
	if err := (ConfigService{Out: &sb}).Show(config); err != nil {
		t.Fatalf("Show: %v", err)
	}

	if strings.Contains(sb.String(), `\n`) {
		t.Errorf("output contains a literal backslash-n:\n%s", sb.String())
	}
}
