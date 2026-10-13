package msg

import (
	"fmt"

	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/message_info"
)

// A message's spam status is two independent records, each with one writer:
//
//   - Evidence, written only by the scan: which checks fired, bogofilter's
//     score, and the stamp of the filter and settings it was computed against.
//   - Override, written only by the user: ham or spam, or none.
//
// Whether a message is spam is derived from the two and never stored. They
// used to be one verdict, which every move, re-scan and inbox re-label had to
// clear without losing the user's decision. A path that missed that rule --
// the move after an unspam -- relabelled mail the user had just called ham.
// With separate records there is nothing to clear: a scan cannot touch the
// override, and an override outranks whatever the evidence says.

// Override is the user's decision about a message.
type Override uint8

// The user's decision about a message: OverrideNone when they made none,
// otherwise ham or spam. It outranks whatever the scan's evidence says.
const (
	OverrideNone = Override(message_info.OverrideNone)
	OverrideHam  = Override(message_info.OverrideHam)
	OverrideSpam = Override(message_info.OverrideSpam)
)

// Checks is the set of spam checks that fired on a message.
type Checks uint8

const (
	// CheckBogofilter reports that bogofilter judged the message spam.
	CheckBogofilter Checks = 1 << iota
	// CheckBlacklist reports that the sender is on the blacklist.
	CheckBlacklist
	// CheckLanguage reports a script or language outside allowed_languages.
	CheckLanguage
	// CheckAdvert reports that the subject marks the message as an advertisement.
	CheckAdvert
	// CheckPolitical reports political fundraising (donation links, PAC disclaimers).
	CheckPolitical
	// CheckWhitelist reports that the sender is whitelisted, which outranks
	// every other check.
	CheckWhitelist
)

// spamChecks are the checks that make a message spam on their own.
const spamChecks = CheckBogofilter | CheckBlacklist | CheckLanguage | CheckAdvert | CheckPolitical

// Evidence is what one scan found.
type Evidence struct {
	Checks    Checks
	BogoScore float32
	Stamp     uint32
}

// IsSpam reports whether the message counts as spam: the user's override if
// they made one, otherwise whether any spam check fired.
func (i *Info) IsSpam() bool {
	switch i.Override {
	case OverrideSpam:
		return true
	case OverrideHam:
		return false
	}
	if !i.Scanned || i.Evidence.Checks&CheckWhitelist != 0 {
		return false
	}
	return i.Evidence.Checks&spamChecks != 0
}

// IsPolitical reports political fundraising the user has not called ham.
func (i *Info) IsPolitical() bool {
	return i.flagged(CheckPolitical)
}

// IsBlacklisted reports a blacklisted sender the user has not called ham.
func (i *Info) IsBlacklisted() bool {
	return i.flagged(CheckBlacklist)
}

func (i *Info) flagged(c Checks) bool {
	return i.Override != OverrideHam && i.Scanned && i.Evidence.Checks&c != 0
}

// NeedsScan reports whether the message must be scanned against stamp: it
// has no override, and its evidence is missing or was computed against
// another filter or other settings.
func (i *Info) NeedsScan(stamp uint32) bool {
	return i.Override == OverrideNone && (!i.Scanned || i.Evidence.Stamp != stamp)
}

// SetEvidence records what a scan found, leaving the override alone.
func SetEvidence(account cachedir.Account, msgID email.ID, ev Evidence) error {
	return updateCompact(account, msgID, func(c *message_info.Compact) {
		c.Flags |= message_info.FlagScanned
		c.Checks = uint8(ev.Checks)
		c.BogoScore = ev.BogoScore
		c.Stamp = ev.Stamp
	})
}

// SetOverride records the user's decision, leaving the evidence alone.
func SetOverride(account cachedir.Account, msgID email.ID, o Override) error {
	return updateCompact(account, msgID, func(c *message_info.Compact) {
		c.Override = uint8(o)
	})
}

// ForgetEvidence drops a message's scan evidence, so the next scan judges it
// afresh; the user's override is kept.
func ForgetEvidence(account cachedir.Account, msgID email.ID) error {
	c := getCompact(account.Path(), msgID)
	if c == nil || c.Flags&message_info.FlagScanned == 0 {
		return nil
	}
	return setCompact(account.Path(), msgID, c.WithoutEvidence())
}

func updateCompact(account cachedir.Account, msgID email.ID, edit func(*message_info.Compact)) error {
	c := getCompact(account.Path(), msgID)
	if c == nil {
		return fmt.Errorf("message %s not in cache", msgID)
	}
	updated := *c
	edit(&updated)
	if updated == *c {
		return nil
	}
	return setCompact(account.Path(), msgID, updated)
}
