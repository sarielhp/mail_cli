package app

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// PrintError prints a formatted error message to os.Stderr with a separator line
// and "Error: " in bold red.
func PrintError(err error) {
	FprintError(os.Stderr, err)
}

// PrintErrorf formats and prints an error message to os.Stderr with a separator line
// and "Error: " in bold red.
func PrintErrorf(format string, args ...any) {
	FprintErrorf(os.Stderr, format, args...)
}

// FprintError prints a separator line and formatted red "Error: " message to w.
func FprintError(w io.Writer, err error) {
	if err == nil {
		return
	}
	FprintErrorMessage(w, err.Error())
}

// FprintErrorf formats and prints a separator line and red "Error: " message to w.
func FprintErrorf(w io.Writer, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	FprintErrorMessage(w, msg)
}

// FprintErrorMessage prints the separator line and red "Error: " followed by msg to w.
func FprintErrorMessage(w io.Writer, msg string) {
	sep := strings.Repeat("─", 70)
	cleanMsg := strings.TrimPrefix(msg, "Error: ")
	cleanMsg = strings.TrimPrefix(cleanMsg, "error: ")
	fmt.Fprintf(w, "\n%s\n%s %s\n", sep, ColorBoldRed.Sprint("Error:"), cleanMsg)
}
