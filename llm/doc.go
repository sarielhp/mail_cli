// Package llm summarises a message through an OpenAI-compatible chat endpoint.
//
// The call is bounded twice, by a context deadline and by the HTTP client's own
// timeout, because an unreachable or hung endpoint must not wedge a scan that
// merely offered to summarise something.
package llm
