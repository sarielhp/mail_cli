// Package usage owns the generated documentation: the per-command Markdown tree
// under docs/clihelp, and the package inventory inside docs/ARCHITECTURE.md.
//
// Both are generated rather than written because both had drifted -- the
// command pages by three stale files and a version string, the architecture
// document into describing a flat layout that had not existed for a long time.
// go run . docs regenerates them, and drift tests fail if what is
// committed no longer matches.
package usage
