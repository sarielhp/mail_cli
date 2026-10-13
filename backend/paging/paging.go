// Package paging converges a batched walk over a result set that the batch
// function itself is shrinking.
//
// The bulk spam operations used to issue one request with a fixed limit and no
// loop, then report "Successfully purged N" -- so a mailbox holding more than
// that was partly processed and wholly reported. The residue then fed back into
// every later run. This is the loop that walks it to the end, shared by the
// backends that page through a mailbox by ID.
package paging

import "fmt"

// Converge runs next and process until the result set is empty or a pass makes
// no progress.
//
// Each pass re-reads from the start rather than advancing a cursor, because
// process removes what it dealt with: the positions behind a cursor shift under
// it, and a page boundary is exactly where that loses entries. Re-reading
// converges instead.
//
// process returns how many entries it actually dealt with. A pass that returns
// zero while the source is still listing entries is not making progress -- an
// entry the server will not process would otherwise spin here forever -- so the
// loop stops and says what is left.
func Converge[ID any](next func() ([]ID, error), process func([]ID) (int, error)) (int, error) {
	total := 0
	for {
		ids, err := next()
		if err != nil {
			return total, err
		}
		if len(ids) == 0 {
			return total, nil
		}

		done, err := process(ids)
		total += done
		if err != nil {
			return total, err
		}
		if done == 0 {
			return total, fmt.Errorf("stopped with %d message(s) the server would not process", len(ids))
		}
	}
}
