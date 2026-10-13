package app

import "github.com/sarielhp/clihelp"

// The global flag values the CLI parses into: FlagAccountName names the
// account a command targets, the booleans select modes such as --verbose and
// --dry-run, and CmdExecutionStarted records that a command has begun.
var (
	FlagAccountName string

	FlagVerbose   bool
	FlagVersion   bool
	FlagLogAppend bool
	FlagFixConfig bool
	FlagReadOnly  bool
	FlagDryRun    bool
	FlagJSON      bool

	CmdExecutionStarted bool
)

// ErrUsage marks an error as a command-line mistake, which main turns into exit
// code 2 rather than 1.
var ErrUsage = clihelp.ErrUsage
