// Package runner orchestrates whole commands: the account-spanning scan, show
// and last, rule import and export, and the service types that implement the
// interfaces cli and tui declare.
//
// It sits above mailops, which it uses to obtain clients, and it may import the
// user interface -- launching the TUI is one of the things it orchestrates.
// Nothing here is imported by a layer below it; main wires these services to
// the interfaces the CLI asks for.
package runner
