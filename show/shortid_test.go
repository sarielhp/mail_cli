package show

import "github.com/sarielhp/mail_cli/email"

// emailComputeShortID mirrors what listings print beside each message, so the
// test can build the short form the same way the code matches it.
func emailComputeShortID(id string) string {
	return string(email.ComputeShortID(email.ID(id)))
}
