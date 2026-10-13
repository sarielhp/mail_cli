package show

// Options is what a show was asked to do.
//
// Web arrived as the package-level app.FlagShowWeb, written by the CLI's flag
// parser and read here.
type Options struct {
	// Web opens the HTML body in a browser instead of printing plain text.
	Web bool
	// Raw writes the raw RFC822 bytes directly to output.
	Raw bool
	// JSON outputs email or folder details as machine-readable JSON.
	JSON bool
}
