package usage

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// docsDir is the committed reference documentation, relative to this package.
const docsDir = "../docs/clihelp"

// generatedFiles renders the CLI's documentation into a temporary directory and
// returns it as filename -> contents.
func generatedFiles(t *testing.T) map[string]string {
	t.Helper()

	dir := t.TempDir()
	if _, err := RenderMarkdown(MarkdownOptions{Dir: dir}); err != nil {
		t.Fatalf("rendering documentation: %v", err)
	}
	return readMarkdown(t, dir)
}

func readMarkdown(t *testing.T, dir string) map[string]string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}

	out := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", e.Name(), err)
		}
		out[e.Name()] = string(data)
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// docs/clihelp is generated, committed, and read by people who are not running
// the binary. Nothing regenerates it automatically, so without this test it
// drifts silently -- which it had, by three pages left behind when their
// commands became hidden, and by a version string and two flag spellings.
func TestDocsAreNotStale(t *testing.T) {
	want := generatedFiles(t)
	got := readMarkdown(t, docsDir)

	for _, name := range sortedKeys(want) {
		committed, ok := got[name]
		if !ok {
			t.Errorf("docs/clihelp/%s is missing; run go run . docs", name)
			continue
		}
		if committed != want[name] {
			t.Errorf("docs/clihelp/%s is out of date; run go run . docs", name)
		}
	}

	for _, name := range sortedKeys(got) {
		if _, ok := want[name]; !ok {
			t.Errorf("docs/clihelp/%s is no longer generated; delete it", name)
		}
	}
}

// A page per command, and the index. If the generator silently stopped emitting
// most of the tree the test above would pass on whatever was left.
func TestDocsCoverTheCommandTree(t *testing.T) {
	want := generatedFiles(t)

	if len(want) < 50 {
		t.Fatalf("expected the generator to emit a page per command, got %d", len(want))
	}
	for _, required := range []string{"index.md", "scan.md", "msg-show.md", "write.md", "tui.md"} {
		if _, ok := want[required]; !ok {
			t.Errorf("%s was not generated", required)
		}
	}
}
