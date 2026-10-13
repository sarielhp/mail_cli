// Package cli is the command-line front end: the command tree, the flags, and
// the translation from what the user typed into a call on an operation.
//
// It declares the interfaces it needs -- MailOps, SpamOps, RuleOps, ConfigOps
// and their neighbours in deps.go -- rather than importing the package that
// implements them. runner satisfies them and main binds the two, so neither
// side changes when the other does and these tests run with stubs and no
// backend in the process.
//
// Nothing here does mail work. A command resolves its arguments, asks a
// session for the operation, and renders the result: the status through
// report.Reporter, the product through an io.Writer. Anything more belongs in
// the domain package that owns it.
package cli
