package cli

import (
	"encoding/json"
	"fmt"

	"github.com/sarielhp/mail_cli/app"
)

// --json turns a listing into something a script can read.
//
// It writes to the product stream and nothing else does: progress, warnings and
// the deprecation notices all go to the status stream, so `mail_cli --json
// label list | jq` sees JSON and only JSON. That separation is what makes the
// flag possible at all -- before it, every one of those lines was on stdout.

// wantJSON reports whether this invocation asked for machine-readable output.
func wantJSON() bool { return app.FlagJSON }

// emitJSON writes v to the product stream, indented, with a trailing newline.
func (s *Session) emitJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding output as JSON: %w", err)
	}
	fmt.Fprintf(s.out(), "%s\n", data)
	return nil
}

// writeJSONList writes a slice as an indented JSON array with a trailing
// newline, rendering a nil slice as [] rather than null.
func writeJSONList[T any](session *Session, v []T) error {
	if v == nil {
		v = []T{}
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, _ = session.out().Write(data)
	fmt.Fprintln(session.out())
	return nil
}
