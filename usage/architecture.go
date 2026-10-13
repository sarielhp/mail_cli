package usage

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// The package inventory in docs/ARCHITECTURE.md is generated rather than
// written, for the reason the file itself demonstrates: the hand-written
// version described a flat `package main` layout with about twenty-six files
// that no longer existed, and instructed the reader never to create
// subdirectories -- against a tree of thirty packages. A document that is
// maintained by hand and read by people who are not compiling is a document
// that drifts, and prose cannot be tested. This section can.
const (
	archBeginMarker = "<!-- BEGIN GENERATED: package inventory -->"
	archEndMarker   = "<!-- END GENERATED: package inventory -->"
)

type listedPackage struct {
	ImportPath   string
	Doc          string
	GoFiles      []string
	TestGoFiles  []string
	XTestGoFiles []string
	// Module is the module the package belongs to; its path is what the
	// inventory strips to name a package by its directory.
	Module struct{ Path string }
}

// listPackages asks the toolchain what the tree actually contains.
func listPackages(dir string) ([]listedPackage, error) {
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}

	var pkgs []listedPackage
	dec := json.NewDecoder(strings.NewReader(string(out)))
	for dec.More() {
		var p listedPackage
		if err := dec.Decode(&p); err != nil {
			return nil, fmt.Errorf("decoding go list output: %w", err)
		}
		pkgs = append(pkgs, p)
	}
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].ImportPath < pkgs[j].ImportPath })
	return pkgs, nil
}

// RenderPackageInventory returns the generated table of packages.
func RenderPackageInventory(dir string) (string, error) {
	pkgs, err := listPackages(dir)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", archBeginMarker)
	b.WriteString("*Generated from `go list -json ./...`. Do not edit by hand; run `go run . docs`*\n\n")
	fmt.Fprintf(&b, "%d packages.\n\n", len(pkgs))
	b.WriteString("| Package | Files | Purpose |\n|---|---:|---|\n")

	for _, p := range pkgs {
		name := strings.TrimPrefix(p.ImportPath, p.Module.Path)
		name = strings.TrimPrefix(name, "/")
		if name == "" {
			name = "." // the main package
		}
		doc := strings.TrimSpace(p.Doc)
		if doc == "" {
			doc = "—"
		}
		doc = strings.ReplaceAll(doc, "|", `\|`)
		fmt.Fprintf(&b, "| `%s` | %d | %s |\n", name, len(p.GoFiles), doc)
	}

	fmt.Fprintf(&b, "\n%s", archEndMarker)
	return b.String(), nil
}

// ApplyPackageInventory rewrites the generated block of path in place and
// reports whether anything changed.
func ApplyPackageInventory(dir, path string) (bool, error) {
	existing, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}

	block, err := RenderPackageInventory(dir)
	if err != nil {
		return false, err
	}

	updated, err := replaceBlock(string(existing), block)
	if err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	if updated == string(existing) {
		return false, nil
	}
	return true, os.WriteFile(path, []byte(updated), 0o644)
}

// replaceBlock swaps whatever lies between the markers for block.
func replaceBlock(doc, block string) (string, error) {
	start := strings.Index(doc, archBeginMarker)
	end := strings.Index(doc, archEndMarker)
	if start < 0 || end < 0 || end < start {
		return "", fmt.Errorf("the generated markers are missing or out of order")
	}
	return doc[:start] + block + doc[end+len(archEndMarker):], nil
}
