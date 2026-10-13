package scan

// Options is what a scan was asked to do, as opposed to how the account is
// configured.
//
// These arrived as package-level globals in app -- opts.Pattern and
// opts.ExplicitInbox -- written by the CLI's flag parser and read here.
// That made a scan impossible to run with different options from the TUI, or
// twice with different options in one test, without writing to a global.
type Options struct {
	// Pattern, when set, restricts the scan to messages whose subject matches.
	Pattern string

	// ExplicitInbox records that the user named the inbox rather than reaching
	// it by default, which is what licenses clearing cached classifications.
	ExplicitInbox bool

	// MoveSpam moves every message classified as spam into the Spam folder.
	MoveSpam bool

	// MoveFrom moves a single message from this sender into the Spam folder,
	// and only when exactly one message from them is found.
	//
	// MoveSpam and MoveFrom used to be one string flag with an optional value,
	// where the absent value was recorded as the literal "true" -- a boolean
	// smuggled through a field typed as an email address, which every reader
	// downstream had to know about.
	MoveFrom string

	// MoveInbox moves messages from this sender out of Spam and back to Inbox.
	MoveInbox string

	// JSON suppresses the human table and makes Perform return a
	// machine-readable FolderReport per scanned folder, which the caller
	// serializes. The reports are what a script reads; the progress the scan
	// narrates stays on the status channel regardless.
	JSON bool
}
