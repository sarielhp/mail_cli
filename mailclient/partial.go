package mailclient

import (
	"errors"
	"fmt"
	"github.com/sarielhp/mail_cli/email"
	"strings"
)

// PartialMoveError reports that a batch move partly succeeded.
//
// A batch backend does not fail as a unit. JMAP's Email/set returns a
// per-message notUpdated map, so a request for forty messages can move
// thirty-eight and refuse two -- and the refusal used to be flattened into one
// error naming the first offender. Every caller then read that as "nothing
// happened" and skipped reconciliation for the whole batch, which left the
// thirty-eight that did move listed in their old folder until something else
// refreshed the index.
//
// Failed names the messages the server refused. Everything else in the batch is
// on the server's new side of the move and must be reconciled locally, even
// though the operation as a whole reports an error.
type PartialMoveError struct {
	Failed []email.ID
	Err    error
}

func (e *PartialMoveError) Error() string {
	if len(e.Failed) == 1 {
		return fmt.Sprintf("%v (1 message not moved: %s)", e.Err, e.Failed[0])
	}
	return fmt.Sprintf("%v (%d messages not moved: %s)", e.Err, len(e.Failed), strings.Join(email.IDStrings(e.Failed), ", "))
}

func (e *PartialMoveError) Unwrap() error { return e.Err }

// movedDespite returns the IDs of a batch that are not named as failures.
//
// With no PartialMoveError to consult, nothing is assumed to have moved: an
// error from a backend that cannot report per-message outcomes might mean the
// request never reached the server at all.
func movedDespite(all []email.ID, err error) []email.ID {
	var partial *PartialMoveError
	if !errors.As(err, &partial) {
		return nil
	}
	failed := make(map[email.ID]bool, len(partial.Failed))
	for _, id := range partial.Failed {
		failed[id] = true
	}
	moved := make([]email.ID, 0, len(all))
	for _, id := range all {
		if !failed[id] {
			moved = append(moved, id)
		}
	}
	return moved
}
