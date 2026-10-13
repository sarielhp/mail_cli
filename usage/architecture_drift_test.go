package usage

import (
	"os"
	"strings"
	"testing"
)

const architecturePath = "../docs/ARCHITECTURE.md"

// docs/ARCHITECTURE.md is read by people who are not compiling, which is how it
// came to describe a flat `package main` layout, list about twenty-six files
// that did not exist, and instruct the reader never to create subdirectories --
// against a tree of forty-one packages. Prose cannot be tested; the inventory
// can, so the inventory is generated and this keeps it current.
func TestArchitectureInventoryIsNotStale(t *testing.T) {
	committed, err := os.ReadFile(architecturePath)
	if err != nil {
		t.Fatalf("reading %s: %v", architecturePath, err)
	}

	block, err := RenderPackageInventory("..")
	if err != nil {
		t.Fatalf("generating the inventory: %v", err)
	}

	want, err := replaceBlock(string(committed), block)
	if err != nil {
		t.Fatalf("%s: %v", architecturePath, err)
	}
	if want != string(committed) {
		t.Errorf("%s is out of date; run go run . docs", architecturePath)
	}
}

// The generated block is only worth keeping current if it is actually there and
// actually describes this program. A generator that silently emitted an empty
// table would satisfy the drift test above on whatever was committed.
func TestArchitectureInventoryDescribesTheTree(t *testing.T) {
	block, err := RenderPackageInventory("..")
	if err != nil {
		t.Fatalf("generating the inventory: %v", err)
	}

	for _, pkg := range []string{"`cli`", "`tui`", "`runner`", "`mailops`", "`mailclient`", "`report`"} {
		if !strings.Contains(block, "| "+pkg+" |") {
			t.Errorf("the inventory does not list %s", pkg)
		}
	}
	if n := strings.Count(block, "\n| `"); n < 30 {
		t.Errorf("the inventory lists %d packages; the tree has many more", n)
	}
}

// The layering rules the document describes are enforced by
// tools/arch_check.rb, and the document says so. If the checker is ever
// removed, the claim becomes false with nothing to catch it.
func TestArchitectureCheckerStillExists(t *testing.T) {
	if _, err := os.Stat("../tools/arch_check.rb"); err != nil {
		t.Errorf("docs/ARCHITECTURE.md cites tools/arch_check.rb as the enforcement: %v", err)
	}
}
