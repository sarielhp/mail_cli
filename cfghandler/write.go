package cfghandler

import (
	"fmt"
	"io"
	"strings"
)

// fprintfPrefixed writes one marked line.
//
// The config commands report to a writer rather than a report.Reporter: their
// output *is* what was asked for, so `mail_cli config show > settings.txt`
// should capture it. The markers are kept because the report reads better with
// them.
func fprintfPrefixed(w io.Writer, prefix, format string, a ...any) {
	fmt.Fprintf(w, "%s %s\n", prefix, strings.TrimRight(fmt.Sprintf(format, a...), "\n"))
}
