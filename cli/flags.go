package cli

import (
	"time"

	"github.com/sarielhp/clihelp"
)

// Flag variables bound by clihelp when it parses a command line.
//
// They live here rather than in app because they are the CLI's own parse state:
// app is a leaf utility package, and having it hold the flags meant scan and
// show could reach in and read them, which is how a scan became impossible to
// run with different options from the TUI or twice in one test. Anything a
// layer below the CLI needs is passed to it as an argument -- scan.Options,
// show.Options -- not read from here.
var (
	flagAccountIndex string
	flagMoveSpam     bool
	flagMoveSpamFrom string
	flagMoveInboxStr string
	flagScanPattern  string
	flagConfigFix    bool
	flagLabelRmYes   bool
	flagShowWeb      bool
	flagShowRaw      bool

	flagSendSubject string
	flagSendCc      string
	flagSendBcc     string
	flagSendAttach  []string
	flagSendInclude string
	flagSendBody    string
	flagSendRaw     bool
	flagSendQueue   bool
	flagSendSign    bool
	flagSendEncrypt bool
	flagSendPgpKey  string

	flagComposeSubject       string
	flagComposeCc            string
	flagComposeBcc           string
	flagComposeAttach        []string
	flagComposeInclude       string
	flagComposeBody          string
	flagComposeResume        string
	flagDraftsPruneSent      bool
	flagDraftsPruneBySubject bool

	flagReplySubject  string
	flagReplyCc       string
	flagReplyBcc      string
	flagReplyAttach   string
	flagReplyInclude  string
	flagReplyBody     string
	flagReplyGroup    bool
	flagReplyHeadless bool
	flagReplyNoQuote  bool

	flagExportOutput string
	flagExportFormat string
	flagExportSingle bool

	flagRuleExportFile  string
	flagRuleImportFile  string
	flagRuleListAll     bool
	flagSieveExport     string
	flagLabelsListAll   bool
	flagForceLearnHam   bool
	flagForceRuleExport bool
	flagBlockTrash      bool
	flagBlockPush       bool

	// Per-command confirmations for the irreversible operations. Not persistent:
	// --yes means something only where there is something to confirm.
	flagSpamDelYes          bool
	flagCacheResetYes       bool
	flagRuleDeleteAllYes    bool
	flagAccountDeleteYes    bool
	flagRuleRemoteRmYes     bool
	flagRuleRemoteKeepLocal bool
	flagLearnHamFolder      string

	// The watcher's own parse state. --interval is a duration, so it is a
	// time.Duration and not a string the command re-parses. The account to
	// watch comes from the global -A/--account, so there is no flag here for
	// it: declaring one would duplicate the persistent option.
	flagWatchInterval time.Duration
	flagWatchOnMail   string
	flagWatchNoNotify bool
	flagWatchNoFlush  bool
)

// patternOption is scan's subject filter. It is declared by every command that
// honours it -- the root's bare scan form, `scan`, and `label sort` -- rather
// than persistently, because a flag on every command that three of them read is
// a flag the other fifty-odd advertise in help and silently ignore.
func patternOption(target *string) clihelp.Option {
	return clihelp.Group("Execution Modes", clihelp.String(target, "-p, --pattern <pattern>", "",
		"Only messages whose subject has this"))
}

// scanOptions is the flag set the scan helper honours, declared once and placed
// in both scopes that run it: the `scan` command and the root's bare `%label`
// form. clihelp binds options per scope, so sharing the descriptors -- as
// patternOption already is between root, scan and splice -- keeps the two
// spellings from drifting; declaring them only on `scan` is what made
// `mail_cli % -m` an unknown-flag error while scanLabelRef still read them.
func scanOptions() []clihelp.Option {
	return []clihelp.Option{
		patternOption(&flagScanPattern),
		clihelp.Bool(&flagMoveSpam, "-m, --move", false,
			"Move identified spam emails to the Spam folder"),
		clihelp.String(&flagMoveSpamFrom, "--move-from <From>", "",
			"Move the lone message from this sender to Spam"),
		clihelp.String(&flagMoveInboxStr, "--inbox-move <From>", "",
			"Move mail from this sender back to Inbox"),
	}
}
