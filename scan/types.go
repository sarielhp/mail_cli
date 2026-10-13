package scan

import "github.com/sarielhp/mail_cli/spam"

// The classifier's verdict, named locally for readability at the call sites
// that pass it around.
type (
	// Thresholds are the score boundaries the classifier was configured with.
	Thresholds = spam.Thresholds
	// Symbol is one named rule that fired, and what it contributed to the score.
	Symbol = spam.Symbol
	// Response is the classifier's verdict on a single message.
	Response = spam.Response
)
