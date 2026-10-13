package cli

import (
	"bytes"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/label"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"

	"github.com/sarielhp/clihelp"
)

var (
	maxConfirmRetries = 5
	confirmRetryDelay = 2 * time.Second
)

var (
	flagSpliceMove         bool
	flagSpliceCopy         bool
	flagSpliceN            int
	flagSpliceFolder       string
	flagSpliceFolderSuffix string
	flagSpliceFolderYear   string
	flagSplicePattern      string
	flagSpliceAllow        bool
)

func spliceFolderOption(target *string, flags, usage string) clihelp.Option {
	opt := clihelp.String(target, flags, "", usage)
	opt.Complete = CompleteFolderNames
	return opt
}

// SpliceCmd returns the clihelp.Command for the splice command.
func spliceCmd(session *Session, name, path string) clihelp.Command {
	return clihelp.Command{
		Name:            name,
		Group:           "Folders",
		Description:     "Organize messages into dated keep/ hierarchy.",
		LongDescription: "Organize and archive messages from a source folder/label into the date-based keep/YYYY/MM/<folder> structure (root \"keep/\" is fixed). Runs in dry-run mode by default; use --move to perform actual moves or --copy to copy without deleting from source. Use -f, -F, or -Y to customize destination folder naming.",
		UsageLine:       "mail_cli " + path + " <folder> [flags]\nmail_cli " + path + " examples",
		Parameters: []clihelp.Param{
			{Name: "<folder>", Description: "Source folder/label name (e.g. \"receipts\" or \"GMail:receipts\").", Complete: CompleteLabelRefs},
		},
		Options: []clihelp.Option{
			clihelp.Bool(&flagSpliceMove, "--move", false, "Move the messages (default: dry run)"),
			clihelp.Bool(&flagSpliceCopy, "--copy", false, "Copy the messages (default: dry run)"),
			clihelp.Int(&flagSpliceN, "-n <num>", 10, "Number of messages to process (default: 10)"),
			patternOption(&flagSplicePattern),
			spliceFolderOption(&flagSpliceFolder, "-f, --folder <name>", "Folder keep/YYYY/MM/<name>"),
			spliceFolderOption(&flagSpliceFolderSuffix, "-F, --folder-suffix <name>", "Folder keep/YYYY/MM/<name>-YYYY-MM"),
			spliceFolderOption(&flagSpliceFolderYear, "-Y, --folder-year <name>", "Folder keep/YYYY/<name>-YYYY-MM"),
			clihelp.Bool(&flagSpliceAllow, "--allow", false, "Allow sourcing from folders that start with keep/"),
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli " + path + " %receipts", Description: "Dry run preview of date-based archive for receipts folder"},
			{Line: "mail_cli " + path + " %receipts --move", Description: "Move receipts messages to keep/YYYY/MM/receipts"},
			{Line: "mail_cli " + path + " %receipts -f Archive --move", Description: "Move receipts messages to keep/YYYY/MM/Archive"},
			{Line: "mail_cli " + path + " %receipts -F Receipts --move", Description: "Move receipts messages to keep/YYYY/MM/Receipts-YYYY-MM"},
			{Line: "mail_cli " + path + " %GMail:received -Y %FastMail:services --move", Description: "Cross-account move from GMail to FastMail keep/YYYY/services-YYYY-MM"},
			{Line: "mail_cli " + path + " %receipts -n 50 --copy", Description: "Copy up to 50 receipts messages without deleting source"},
			{Line: "mail_cli " + path + ` %receipts -p "*order*" --move`, Description: "Move only receipts messages matching pattern"},
			{Line: "mail_cli " + path + " examples", Description: "Show detailed structured examples and mapping patterns"},
		},
		Subcommands: []clihelp.Command{
			SpliceExamplesCmd(),
		},
		Args: clihelp.ExactArgs(1),
		OptionsValidator: clihelp.ValidateOptions(
			clihelp.MutuallyExclusive("move", "copy"),
			clihelp.MutuallyExclusive("folder", "folder-suffix", "folder-year"),
		),
		Run: func(ctx *clihelp.Context) error {
			return runSplice(ctx, session)
		},
	}
}

func runSplice(ctx *clihelp.Context, session *Session) error {
	defer func() {
		flagSpliceMove = false
		flagSpliceCopy = false
		flagSplicePattern = ""
	}()

	srcClient, source, err := resolveSourceClientAndFolder(session, ctx.Args[0], flagSpliceN, flagSpliceAllow)
	if err != nil {
		return err
	}

	destClient, destBase, yearSuffix, suffix, err := resolveDestClientAndBase(session, srcClient, source)
	if err != nil {
		return err
	}
	plan := &splicePlan{
		session: session, src: srcClient, dest: destClient, source: source,
		destBase: destBase, yearSuffix: yearSuffix, suffix: suffix,
	}

	labelCache := NewSanitizeLabelCache()
	ids, err := plan.fetchIDs(labelCache, flagSpliceN)
	if err != nil || len(ids) == 0 {
		return err
	}
	touched, err := plan.run(ids)
	if err != nil {
		return err
	}
	plan.printTotals(touched, labelCache)
	return nil
}

func resolveSourceClientAndFolder(session *Session, folderArg string, numMessages int, allowKeep bool) (mailclient.MailClient, label.Folder, error) {
	srcClient, srcFolder, err := session.ResolveClientAndLabel(folderArg)
	if err != nil {
		return nil, label.Folder{}, err
	}

	if argsErr := ValidateSpliceArgs(srcFolder, numMessages, allowKeep); argsErr != nil {
		return nil, label.Folder{}, argsErr
	}

	if validateErr := srcClient.Validate(); validateErr != nil {
		return nil, label.Folder{}, validateErr
	}

	source, err := resolveUniqueLabel(srcClient, srcFolder)
	if err != nil {
		srcDisplay := FormatAccountFolder(session, srcClient, srcFolder)
		return nil, label.Folder{}, fmt.Errorf("failed to resolve source folder %s\n  (%s)", srcDisplay, err.Error())
	}
	return srcClient, source, nil
}

func resolveDestClientAndBase(session *Session, srcClient mailclient.MailClient, source label.Folder) (mailclient.MailClient, string, bool, bool, error) {
	destClient := srcClient
	parts := strings.Split(source.String(), "/")
	destFolderBase := parts[len(parts)-1]
	isSuffix := false
	isYearSuffix := false

	// -f, -F and -Y name the destination folder under keep/YYYY/MM/, so a plain
	// name is a plain name -- which is why their completion offers bare names
	// and not the % form. Only a value carrying the sigil is a label reference,
	// and only then can it name another account.
	resolveDest := func(rawSpec string) (mailclient.MailClient, string, error) {
		if !strings.HasPrefix(rawSpec, cfg_g.LabelSigil) {
			return srcClient, rawSpec, nil
		}
		spec, err := cfg_g.ParseLabelRef(rawSpec)
		if err != nil {
			return nil, "", err
		}
		if spec.Account == "" {
			return srcClient, spec.Label, nil
		}
		return session.ResolveClientAndLabel(rawSpec)
	}

	switch {
	case flagSpliceFolderYear != "":
		dc, df, err := resolveDest(flagSpliceFolderYear)
		if err != nil {
			return nil, "", false, false, err
		}
		destClient = dc
		destFolderBase = df
		isYearSuffix = true
	case flagSpliceFolderSuffix != "":
		dc, df, err := resolveDest(flagSpliceFolderSuffix)
		if err != nil {
			return nil, "", false, false, err
		}
		destClient = dc
		destFolderBase = df
		isSuffix = true
	case flagSpliceFolder != "":
		dc, df, err := resolveDest(flagSpliceFolder)
		if err != nil {
			return nil, "", false, false, err
		}
		destClient = dc
		destFolderBase = df
	}

	if err := destClient.Validate(); err != nil {
		return nil, "", false, false, err
	}
	return destClient, destFolderBase, isYearSuffix, isSuffix, nil
}

func parseEmailDate(baseDir cachedir.Account, msgID email.ID) (*email.Email, []byte, string, string, error) {
	rawBytes, err := msg.Read(baseDir, msgID)
	if err != nil {
		return nil, nil, "", "", err
	}
	parsedEmail := email.ParseReader(bytes.NewReader(rawBytes), msgID, "")
	if parsedEmail == nil {
		return nil, nil, "", "", fmt.Errorf("error parsing email %s", msgID)
	}
	if parsedEmail.EmailDate.IsZero() {
		parsedEmail.EmailDate = email.ExtractDateOrNow(rawBytes)
	}
	year := parsedEmail.EmailDate.Format("2006")
	month := parsedEmail.EmailDate.Format("01")
	return parsedEmail, rawBytes, year, month, nil
}

// computeDest builds the destination folder path from the date components and user flags.
func computeDest(year, month string, isYearSuffix, isSuffix bool, destFolderBase string) string {
	if isYearSuffix {
		return "keep/" + year + "/" + destFolderBase + "-" + year + "-" + month
	}
	if isSuffix {
		return "keep/" + year + "/" + month + "/" + destFolderBase + "-" + year + "-" + month
	}
	return "keep/" + year + "/" + month + "/" + destFolderBase
}

// filterByPattern filters message IDs whose subject matches the given pattern.
func filterByPattern(messageIDs []email.ID, baseDir cachedir.Account) []email.ID {
	matcher, err := email.CompilePattern(flagSplicePattern)
	if err != nil {
		slog.Warn("splice: invalid pattern filter, skipping filtering", slog.String("pattern", flagSplicePattern), slog.Any("error", err))
		return messageIDs
	}
	var filteredIDs []email.ID
	for _, msgID := range messageIDs {
		rawBytes, err := msg.Read(baseDir, msgID)
		if err != nil {
			slog.Warn("splice: pattern filter skipping message due to read error", slog.String("msgID", string(msgID)), slog.Any("error", err))
			continue
		}
		parsedEmail := email.ParseReader(bytes.NewReader(rawBytes), msgID, "")
		if parsedEmail == nil {
			continue
		}
		if matcher(parsedEmail.Subject) {
			filteredIDs = append(filteredIDs, msgID)
		}
	}
	return filteredIDs
}

// print helpers for splice status lines

func printSpliceError(w io.Writer, i, total int, msgID email.ID, subject string) {
	fmt.Fprintf(w, "[%d/%d] %s %s | Subject: %q\n", i+1, total, app.PrefixError, msgID, subject)
}

func printSpliceWarn(w io.Writer, i, total int, msgID email.ID, subject string) {
	fmt.Fprintf(w, "[%d/%d] %s %s | Subject: %q\n", i+1, total, app.PrefixWarn, msgID, subject)
}

func printSpliceSuccess(w io.Writer, i, total int, msgID email.ID, subject string) {
	fmt.Fprintf(w, "[%d/%d] %s %s | Subject: %q\n", i+1, total, app.PrefixSuccess, msgID, subject)
}

// LabelSortCmd files a folder into the dated keep/ hierarchy: one source, many
// destinations. "sort" names the shape of the operation, which is what
// distinguishes it from label move -- splice and split were a minimal pair that
// said nothing about the difference.
func LabelSortCmd(session *Session) clihelp.Command {
	return spliceCmd(session, "sort", "label sort")
}
