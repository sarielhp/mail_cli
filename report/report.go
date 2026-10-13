// Package report is how the layers below the user interface say what they are
// doing, without deciding where it goes or what it looks like.
//
// Backends and domain packages used to call fmt.Printf directly -- 250 times in
// backend alone. That meant their output could not be redirected, could not be
// asserted on without swapping os.Stdout, and could not be shown at all inside
// the TUI, whose display it would have corrupted. The TUI's workaround was a
// package-level app.TuiActive flag that muted the domain layer; that flag is
// gone, because a Reporter answers the question it was asking.
//
// A Reporter replaces all of that. The CLI supplies one that writes prefixed,
// coloured lines to a terminal; the TUI supplies one that turns the same calls
// into bubbletea messages, so progress appears in the interface instead of
// being suppressed; tests supply a buffer, or Discard.
//
// It is a leaf: no mail_cli imports, so any layer can reach it.
package report

// Reporter receives progress and status from work in progress.
//
// The four levelled methods carry a marker the implementation chooses ("[*]",
// "[+]", "[!]"); Printf carries none and is for continuation lines and
// already-composed text.
type Reporter interface {
	Infof(format string, a ...any)
	Successf(format string, a ...any)
	Warnf(format string, a ...any)
	Errorf(format string, a ...any)

	// Printf writes the text as given, with no marker added.
	Printf(format string, a ...any)

	// Progress reports position in a countable job. Implementations may render
	// a bar, emit one message per call, or ignore it. current == total means
	// the job is finished.
	Progress(current, total int, label string)
}

// Discard reports nothing. Use it in tests, and wherever a caller genuinely has
// nowhere to put the output -- never as a way to silence something a user
// should see.
func Discard() Reporter { return discard{} }

type discard struct{}

func (discard) Infof(string, ...any)      {}
func (discard) Successf(string, ...any)   {}
func (discard) Warnf(string, ...any)      {}
func (discard) Errorf(string, ...any)     {}
func (discard) Printf(string, ...any)     {}
func (discard) Progress(int, int, string) {}

// Or returns r, or Discard when r is nil, so a caller that was handed nothing
// does not have to guard every call.
func Or(r Reporter) Reporter {
	if r == nil {
		return Discard()
	}
	return r
}

// Prompter is an optional interface implemented by reporters that can present
// interactive multi-line prompts and instructions directly to a user at a terminal.
//
// Reporters whose output is captured (e.g. TUI ProgramReporter, Discard, or buffer)
// do not allow interactive prompting.
type Prompter interface {
	CanPrompt() bool
}

// CanPrompt reports whether r can present an interactive terminal prompt to a user.
func CanPrompt(r Reporter) bool {
	if p, ok := r.(Prompter); ok {
		return p.CanPrompt()
	}
	return false
}
