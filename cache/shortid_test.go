package cache

import "github.com/sarielhp/mail_cli/email"

// shortIDOf mirrors what listings print beside a message, so a test can look
// one up the way a user would type it.
func shortIDOf(id email.ID) string {
	return string(email.ComputeShortID(id))
}
