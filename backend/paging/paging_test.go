package paging

import (
	"errors"
	"strings"
	"testing"
)

// pages returns a fetcher that serves each page in turn, then nothing --
// standing in for a folder that empties as its messages are processed.
func pages(batches ...[]string) func() ([]string, error) {
	i := 0
	return func() ([]string, error) {
		if i >= len(batches) {
			return nil, nil
		}
		b := batches[i]
		i++
		return b, nil
	}
}

// A bulk operation used to fetch one page -- $top=config.Limit, 100 by default,
// with @odata.nextLink never followed -- act on it, and report success. A Junk
// folder holding more than the limit was partly processed and wholly reported.
func TestConvergeProcessesEveryPage(t *testing.T) {
	var seen []string
	total, err := Converge(
		pages([]string{"a", "b"}, []string{"c"}, []string{"d", "e", "f"}),
		func(ids []string) (int, error) {
			seen = append(seen, ids...)
			return len(ids), nil
		})

	if err != nil {
		t.Fatalf("Converge: %v", err)
	}
	if total != 6 {
		t.Errorf("processed %d, want all 6", total)
	}
	if got := strings.Join(seen, ""); got != "abcdef" {
		t.Errorf("saw %q, want every message once in order", got)
	}
}

// A message the server will not process would otherwise be fetched, refused and
// fetched again for ever, since each pass re-queries from the start.
func TestConvergeStopsWhenNoProgressIsMade(t *testing.T) {
	stuck := func() ([]string, error) { return []string{"wedged"}, nil }

	total, err := Converge(stuck, func([]string) (int, error) { return 0, nil })
	if err == nil {
		t.Fatal("a pass that processed nothing must stop the loop, not repeat it")
	}
	if !strings.Contains(err.Error(), "would not process") {
		t.Errorf("the error should say what is left, got %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
}

// Partial progress is still progress: the loop continues, and what was done is
// counted even when the pass also reports an error.
func TestConvergeKeepsTheCountWhenAPassFails(t *testing.T) {
	boom := errors.New("server said no")

	total, err := Converge(
		pages([]string{"a", "b", "c"}),
		func(_ []string) (int, error) { return 2, boom })

	if !errors.Is(err, boom) {
		t.Fatalf("got %v, want the underlying error", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want the 2 that were processed before the failure", total)
	}
}

func TestConvergeOnAnEmptyFolder(t *testing.T) {
	total, err := Converge(pages(), func([]string) (int, error) {
		t.Error("nothing should be processed for an empty folder")
		return 0, nil
	})
	if err != nil || total != 0 {
		t.Errorf("got (%d, %v), want (0, nil)", total, err)
	}
}
