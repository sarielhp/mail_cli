package scan

import (
	"bytes"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/report"
	"log/slog"
	"mime"
	"net/mail"
	"strings"
	"sync"

	"github.com/abadojack/whatlanggo"
)

type heuristicResult struct {
	IsBlockedLang   bool
	BlockedChar     rune
	BlockedLang     string
	IsNlpBlocked    bool
	NlpDetectedLang string
	NlpConfidence   float64
	IsPolitical     bool
	PoliticalScore  float64
	TriggeredPol    []string
	IsAdvertisement bool
}

func checkContentHeuristics(subject, from, body string, config *cfg_g.Config) heuristicResult {
	var res heuristicResult
	res.IsAdvertisement = strings.Contains(subject, "פרסומת")

	res.IsBlockedLang, res.BlockedChar, res.BlockedLang = disallowedScript(subject, config.AllowedLanguages)
	if !res.IsBlockedLang {
		res.IsBlockedLang, res.BlockedChar, res.BlockedLang = disallowedScript(from, config.AllowedLanguages)
	}
	if !res.IsBlockedLang {
		res.IsBlockedLang, res.BlockedChar, res.BlockedLang = disallowedScript(body, config.AllowedLanguages)
	}
	if res.IsBlockedLang {
		return res
	}

	words := strings.Fields(body)
	if len(words) >= 20 {
		cleanedText := cleanTextForNLP(subject + " " + body)
		if len(cleanedText) >= 30 {
			info := whatlanggo.Detect(cleanedText)
			if info.Confidence >= 0.50 {
				langName := info.Lang.String()
				langISO := info.Lang.Iso6391()
				if !isDetectedLanguageWhitelisted(langName, langISO, config.AllowedLanguages) {
					res.IsNlpBlocked = true
					res.NlpDetectedLang = langName
					res.NlpConfidence = info.Confidence
				}
			}
		}
	}
	if res.IsNlpBlocked {
		return res
	}

	if config.BlockPolitical {
		res.IsPolitical, res.PoliticalScore, res.TriggeredPol = email.DetectPolitical(subject, body)
	}
	return res
}

func applyHeuristicSymbols(resp *Response, h heuristicResult) {
	if resp.Symbols == nil {
		resp.Symbols = make(map[string]Symbol)
	}
	if h.IsBlockedLang {
		resp.Score += 20.0
		resp.Action = fmt.Sprintf("reject (Blocked Language Script: %s)", h.BlockedLang)
		symKey := "CUSTOM_LANGUAGE_BLOCK"
		resp.Symbols[symKey] = Symbol{
			Name:        symKey,
			Score:       20.0,
			Description: fmt.Sprintf("Contains blocked language character '%c' (Unicode U+%04X in script: %s)", h.BlockedChar, h.BlockedChar, h.BlockedLang),
		}
		return
	}
	if h.IsNlpBlocked {
		resp.Score += 20.0
		resp.Action = fmt.Sprintf("reject (Unallowed Language NLP: %s)", h.NlpDetectedLang)
		symKey := "CUSTOM_LANGUAGE_NLP_BLOCK"
		resp.Symbols[symKey] = Symbol{
			Name:        symKey,
			Score:       20.0,
			Description: fmt.Sprintf("NLP detected language '%s' (Confidence %.1f%%) which is not in AllowedLanguages", h.NlpDetectedLang, h.NlpConfidence*100),
		}
		return
	}
	if h.IsAdvertisement {
		resp.Score += 20.0
		resp.Action = "reject (Advertisement Keyword Block)"
		symKey := "CUSTOM_ADVERTISEMENT_BLOCK"
		resp.Symbols[symKey] = Symbol{
			Name:        symKey,
			Score:       20.0,
			Description: "Subject contains the blocked advertisement keyword 'פרסומת'",
		}
		return
	}
	if h.IsPolitical {
		resp.Score += 20.0
		resp.Action = "reject (Political Donation Block)"
		symKey := "CUSTOM_POLITICAL_BLOCK"
		resp.Symbols[symKey] = Symbol{
			Name:        symKey,
			Score:       20.0,
			Description: fmt.Sprintf("Weighted score: %.1f/10.0 (triggered: %s)", h.PoliticalScore, strings.Join(h.TriggeredPol, ", ")),
		}
	}
}

// countUncachedEmails is how many messages the run will actually judge, for
// the progress line: those without an override or current evidence, less the
// whitelisted and blacklisted senders, which need no bogofilter run.
func countUncachedEmails(ids []email.ID, cacheDir cachedir.Account, config *cfg_g.Config, stamp uint32) int {
	realScanCount := 0
	for _, id := range ids {
		if info, err := msg.GetInfo(cacheDir, id); err == nil && !info.NeedsScan(stamp) {
			continue
		}
		emailBytes, lErr := msg.Read(cacheDir, id)
		if lErr != nil {
			continue
		}
		localEmail, errMail := mail.ReadMessage(bytes.NewReader(emailBytes))
		if errMail == nil {
			sender := email.ParseEmailAddress(localEmail.Header.Get("From"))
			if cfg_g.IsWhitelisted(sender, config.Whitelist) || cfg_g.IsBlacklisted(sender, config.Blacklist) {
				continue
			}
		}
		realScanCount++
	}
	return realScanCount
}

// printSpamDetails explains a spam verdict: from the scan's symbols for a
// message just judged, or from the stored evidence for one that was not.
func printSpamDetails(rep report.Reporter, resp Response, cached *msg.Info) {
	rep.Printf("      -> Marked as SPAM because:\n")
	if cached != nil {
		printCachedReasons(rep, cached)
		return
	}
	if len(resp.Symbols) > 0 {
		for _, sym := range resp.Symbols {
			rep.Printf("         * %s: %s (score: %.2f)\n", sym.Name, sym.Description, sym.Score)
		}
		return
	}
	rep.Printf("         * Custom/Heuristics match (Action: %s)\n", resp.Action)
}

func printCachedReasons(rep report.Reporter, info *msg.Info) {
	if info.Override == msg.OverrideSpam {
		rep.Printf("         [Cache] You marked this message as spam.\n")
		return
	}
	checks := info.Evidence.Checks
	for _, r := range []struct {
		check msg.Checks
		text  string
	}{
		{msg.CheckBlacklist, "Sender is blacklisted."},
		{msg.CheckLanguage, "Contains a script or language outside allowed_languages."},
		{msg.CheckAdvert, "Subject marks it as an advertisement."},
		{msg.CheckPolitical, "Identified as political spam."},
		{msg.CheckBogofilter, "Bogofilter judged it spam."},
	} {
		if checks&r.check != 0 {
			rep.Printf("         [Cache] %s\n", r.text)
		}
	}
	rep.Printf("         [Cache] Bogofilter spamicity %.2f.\n", info.Evidence.BogoScore)
}

type senderStatus int

const (
	senderNormal senderStatus = iota
	senderWhitelisted
	senderBlacklisted
)

func evaluateSender(rep report.Reporter, msgID email.ID, localEmail *mail.Message, config *cfg_g.Config) senderStatus {
	sender := email.ParseEmailAddress(localEmail.Header.Get("From"))
	if cfg_g.IsWhitelisted(sender, config.Whitelist) {
		if config.Verbose {
			rep.Successf("    Message %s sender '%s' is whitelisted. Bypassing all checks.\n", msgID, sender)
		}
		return senderWhitelisted
	}
	if cfg_g.IsBlacklisted(sender, config.Blacklist) {
		if config.Verbose {
			rep.Warnf("    Message %s sender '%s' is blacklisted. Bypassing Bogofilter check and moving to spam learn folder.\n", msgID, sender)
		}
		return senderBlacklisted
	}
	return senderNormal
}

// scanUncachedMessage runs bogofilter and the content checks on one message
// and returns what fired, with bogofilter's spamicity.
func scanUncachedMessage(emailBytes []byte, localEmail *mail.Message, subject string, config *cfg_g.Config) (*Response, msg.Checks, float32, error) {
	cr, bogoErr := classifyWithBogofilter(emailBytes)
	if bogoErr != nil {
		return nil, 0, 0, bogoErr
	}
	action := "no action"
	if cr.IsSpam {
		action = "reject"
	}
	resp := &Response{
		Action: action,
		Score:  cr.Score,
		Symbols: map[string]Symbol{
			"BOGOFILTER": {
				Name:        cr.Name,
				Score:       cr.Score,
				Description: cr.Description,
			},
		},
	}

	var bodyStr, fromHeader string
	if localEmail != nil {
		bodyStr, _ = email.ExtractPlainBodyText(localEmail)
		fromHeader = localEmail.Header.Get("From")
	}
	bodyStr = email.StripHTML(bodyStr)
	if len(bodyStr) > 8192 {
		bodyStr = bodyStr[:8192]
	}
	h := checkContentHeuristics(subject, fromHeader, bodyStr, config)
	applyHeuristicSymbols(resp, h)
	return resp, contentChecks(h, cr, config), float32(cr.Score), nil
}

// contentChecks turns bogofilter's result and the heuristics into check bits.
// A score_threshold, when set, replaces bogofilter's own spam cutoff.
func contentChecks(h heuristicResult, cr ClassifierResult, config *cfg_g.Config) msg.Checks {
	var c msg.Checks
	bogoSpam := cr.IsSpam
	if config.ScoreThreshold > 0 {
		bogoSpam = cr.Score >= config.ScoreThreshold
	}
	if bogoSpam {
		c |= msg.CheckBogofilter
	}
	if h.IsBlockedLang || h.IsNlpBlocked {
		c |= msg.CheckLanguage
	}
	if h.IsAdvertisement {
		c |= msg.CheckAdvert
	}
	if h.IsPolitical {
		c |= msg.CheckPolitical
	}
	return c
}

type scanCollector struct {
	mu             sync.Mutex
	spamIDs        []email.ID
	blacklistedIDs []email.ID
	scanResults    map[email.ID]*Response
	scanErr        error
}

func (c *scanCollector) recordError(err error) {
	c.mu.Lock()
	if c.scanErr == nil {
		c.scanErr = err
	}
	c.mu.Unlock()
}

func (c *scanCollector) hasError() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.scanErr != nil
}

func (c *scanCollector) recordResult(msgID email.ID, resp *Response, isSpam, isBlacklisted bool) {
	c.mu.Lock()
	c.scanResults[msgID] = resp
	if isSpam {
		c.spamIDs = append(c.spamIDs, msgID)
	}
	if isBlacklisted {
		c.blacklistedIDs = append(c.blacklistedIDs, msgID)
	}
	c.mu.Unlock()
}

// scanRun is one Emails call: what every worker shares.
type scanRun struct {
	rep       report.Reporter
	config    *cfg_g.Config
	stamp     uint32
	collector *scanCollector
}

// scanOne judges one message, unless the user decided it or its evidence is
// current, and records the evidence -- never a verdict.
func (r *scanRun) scanOne(msgID email.ID, dec *mime.WordDecoder) {
	cacheDir := r.config.AccountCache
	if r.config.Verbose {
		r.rep.Infof("    Requesting Bogofilter scan for message %s...\n", msgID)
	}
	prior, _ := msg.GetInfo(cacheDir, msgID)
	if prior != nil && !prior.NeedsScan(r.stamp) {
		r.recordCached(msgID, prior)
		return
	}
	if exists, _ := msg.Exists(cacheDir, msgID); !exists {
		r.collector.recordError(fmt.Errorf("cached email file for %s is missing or empty", msgID))
		return
	}
	emailBytes, err := msg.Read(cacheDir, msgID)
	if err != nil {
		r.collector.recordError(fmt.Errorf("failed to read cached email %s: %w", msgID, err))
		return
	}
	resp, ev, err := r.judge(msgID, emailBytes, prior, dec)
	if err != nil {
		r.collector.recordError(err)
		return
	}
	_ = msg.SetEvidence(cacheDir, msgID, ev)

	verdict := msg.Info{Scanned: true, Evidence: ev}
	if prior != nil {
		verdict.Override = prior.Override
	}
	r.collector.recordResult(msgID, resp, verdict.IsSpam(), verdict.IsBlacklisted())
	if r.config.Verbose {
		r.rep.Successf("    Completed Bogofilter scan for message %s: Score %.2f (Action: %s)\n", msgID, resp.Score, resp.Action)
		if verdict.IsSpam() {
			printSpamDetails(r.rep, *resp, nil)
		}
	}
}

func (r *scanRun) recordCached(msgID email.ID, info *msg.Info) {
	slog.Info("Loaded cached scan result", slog.String("message_id", string(msgID)))
	act := "no action"
	if info.IsSpam() {
		act = "reject"
	}
	resp := &Response{Action: act, Score: float64(info.Evidence.BogoScore)}
	r.collector.recordResult(msgID, resp, info.IsSpam(), info.IsBlacklisted())
	if r.config.Verbose {
		r.rep.Successf("    Completed Bogofilter scan for message %s: Score %.2f (Action: %s)\n", msgID, resp.Score, resp.Action)
		if info.IsSpam() {
			printSpamDetails(r.rep, *resp, info)
		}
	}
}

// judge decides one message: the sender lists first, then bogofilter and the
// content checks. A whitelisted message is trained into bogofilter as ham the
// first time it is judged; it used to be retrained on every scan.
func (r *scanRun) judge(msgID email.ID, emailBytes []byte, prior *msg.Info, dec *mime.WordDecoder) (*Response, msg.Evidence, error) {
	ev := msg.Evidence{Stamp: r.stamp}
	localEmail, errMail := mail.ReadMessage(bytes.NewReader(emailBytes))
	if errMail != nil {
		localEmail = nil
	}
	var subject string
	if localEmail != nil {
		subject = email.DecodeHeader(dec, localEmail.Header.Get("Subject"))
		switch evaluateSender(r.rep, msgID, localEmail, r.config) {
		case senderWhitelisted:
			if prior == nil || prior.Evidence.Checks&msg.CheckWhitelist == 0 {
				TrainHamLocally(msgID, emailBytes, r.config)
			}
			ev.Checks = msg.CheckWhitelist
			return &Response{Action: "no action (Whitelisted Sender)"}, ev, nil
		case senderBlacklisted:
			ev.Checks = msg.CheckBlacklist
			return &Response{Action: "reject (Blacklisted Sender)", Score: 20.0}, ev, nil
		}
	}
	resp, checks, bogo, err := scanUncachedMessage(emailBytes, localEmail, subject, r.config)
	if err != nil {
		return nil, ev, err
	}
	ev.Checks, ev.BogoScore = checks, bogo
	return resp, ev, nil
}

// dedupeIDs drops repeated message IDs, preserving first-seen order.
//
// Emails dispatches one goroutine per ID; scanOne's
// "already classified" check and its eventual classification write are not
// atomic with each other, so two workers given the same ID would both pass
// the check, both redo the classification work, and both append their
// result to the same output slice. A caller that merges IDs from more than
// one source -- a virtual/search result set alongside a folder listing, or a
// backend returning the same ID across pages -- can hand Emails such a
// duplicate, so it is deduplicated once here rather than trusting every
// caller to do it.
func dedupeIDs(ids []email.ID) []email.ID {
	seen := make(map[email.ID]bool, len(ids))
	out := make([]email.ID, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// Emails scans ids and returns the IDs judged spam, the IDs from blacklisted
// senders, and each scanned message's full verdict. Duplicate IDs are scanned
// once, and any scan error aborts the batch and is returned with no results.
func Emails(rep report.Reporter, ids []email.ID, config *cfg_g.Config, _ string) ([]email.ID, []email.ID, map[email.ID]*Response, error) {
	ids = dedupeIDs(ids)
	collector := &scanCollector{
		scanResults: make(map[email.ID]*Response),
	}

	cacheDir := config.AccountCache
	run := &scanRun{rep: rep, config: config, stamp: Stamp(config), collector: collector}
	realScanCount := countUncachedEmails(ids, cacheDir, config, run.stamp)
	if realScanCount > 0 {
		rep.Infof("Scanning %d email(s) with Bogofilter in parallel...\n", realScanCount)
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)

	for _, id := range ids {
		if collector.hasError() {
			break
		}
		wg.Add(1)
		go func(msgID email.ID) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			run.scanOne(msgID, new(mime.WordDecoder))
		}(id)
	}

	wg.Wait()

	if collector.scanErr != nil {
		return nil, nil, nil, collector.scanErr
	}

	return collector.spamIDs, collector.blacklistedIDs, collector.scanResults, nil
}
