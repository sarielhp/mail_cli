package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_g"

	"github.com/sarielhp/clihelp"
)

// A script needs to tell "I asked for something impossible" from "the server
// refused"; they are different things to retry.
func TestExitCodeFor(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"a usage error", app.ErrUsage, exitUsageError},
		{"a clihelp usage error", clihelp.ErrUsage, exitUsageError},
		{"a wrapped clihelp usage error", fmt.Errorf("%w: unknown flag --bogus", clihelp.ErrUsage), exitUsageError},
		{"a wrapped usage error", fmt.Errorf("%w: too many arguments", app.ErrUsage), exitUsageError},
		{"a doubly wrapped usage error",
			fmt.Errorf("running command: %w", fmt.Errorf("%w: bad count", app.ErrUsage)), exitUsageError},
		{"an unknown account",
			&cfg_g.UnknownAccountError{Ident: "nonesuch", Available: []string{"work"}}, exitUsageError},
		{"a wrapped unknown account",
			fmt.Errorf("resolving: %w", &cfg_g.UnknownAccountError{Ident: "x"}), exitUsageError},
		{"an ambiguous account",
			&cfg_g.AmbiguousAccountError{Ident: "outlook", Candidates: []string{"a", "b"}}, exitUsageError},
		{"a runtime failure", errors.New("server refused the connection"), exitRuntimeError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exitCodeFor(tt.err); got != tt.want {
				t.Errorf("exitCodeFor(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}
