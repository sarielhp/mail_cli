// Package scan is the spam pass over a folder: download what is there, apply
// the account's routing rules, classify the remainder with bogofilter, and move
// what the rules and the classifier agree on.
//
// It takes an Options struct rather than reading the parser's flag globals, so
// it can be run with different options from the TUI, or twice in one test.
// Bogofilter failures are surfaced rather than defaulted to ham -- a missing
// binary used to make the whole classification layer a silent no-op that
// reported success.
package scan
