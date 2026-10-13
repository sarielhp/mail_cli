package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// Confirmation for the things that cannot be undone.
//
// Only irreversible commands ask. A reversible one -- scan applying the routing
// rules you wrote, a message moving between labels -- does not, because a
// confirmation you always type is not a confirmation: it trains the hand to
// answer before the eye reads, and then it protects nothing while costing four
// characters every time.

// ErrDeclined reports that the user answered no. It is not a failure, so
// callers return nil after reporting it.
var ErrDeclined = fmt.Errorf("cancelled")

// Confirm asks before an irreversible action.
//
// With --yes it proceeds. Without a terminal there is nobody to ask, so it
// refuses rather than assuming: a cron line that would have prompted must fail
// loudly, not guess. The refusal names --yes.
func (s *Session) Confirm(action string, assumeYes bool) error {
	if assumeYes {
		return nil
	}

	in := s.stdin()
	if !isTerminal(in) {
		return fmt.Errorf("%w: refusing to %s without --yes, because stdin is not a terminal",
			ErrUsage(), action)
	}

	fmt.Fprintf(s.errOut(), "%s? This cannot be undone. [y/N]: ", action)
	answer, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && err != io.EOF {
		return err
	}

	switch strings.TrimSpace(strings.ToLower(answer)) {
	case "y", "yes":
		return nil
	default:
		return ErrDeclined
	}
}

// stdin is In, defaulting to the process's standard input.
func (s *Session) stdin() io.Reader {
	if s == nil || s.In == nil {
		return os.Stdin
	}
	return s.In
}

// isTerminal reports whether r is an interactive terminal. A reader that is not
// a file never is, which is what makes the tests deterministic.
func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

// declined turns a declined confirmation into a quiet, successful exit.
//
// Answering "no" is not a failure: the user was asked and said no, which is the
// prompt working. Anything else is reported as the error it is.
func declined(session *Session, err error) error {
	if errors.Is(err, ErrDeclined) {
		fmt.Fprintln(session.errOut(), "Cancelled.")
		return nil
	}
	return err
}
