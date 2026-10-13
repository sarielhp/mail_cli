// Package mailops builds backend clients: it resolves an account to its
// per-account config, instantiates the right backend for its type, and wraps it
// in the read-only and caching decorators.
//
// It is deliberately low: its imports stop at the backends, the cache and the
// mailclient interfaces. It used to also hold the orchestration -- the
// account-spanning scan, show and last, and the service types the CLI is wired
// to -- which meant it imported tui, and nothing could depend on the factory
// without pulling the whole interactive client in behind it. That half lives in
// runner now.
package mailops
