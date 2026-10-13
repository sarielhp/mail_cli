// Package integration holds the tests that talk to a real account.
//
// Nothing here runs by default. The read-only suite needs MAIL_CLI_LIVE=1 and
// goes through ReadOnlyMailClient, which refuses or simulates every mutating
// method. The mutation suite needs two further variables and an account
// declared account_type: "test", it only touches folders it created, and it may
// never be extended to exercise spam deletion.
package integration
