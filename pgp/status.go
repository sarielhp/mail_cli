package pgp

import "strings"

// statusPrefix marks a line gpg wrote on its status channel.
const statusPrefix = "[GNUPG:] "

// Outcome is what gpg concluded about one signature.
type Outcome int

// Signature outcomes, from the status token that reported each.
const (
	// OutcomePending is a signature gpg announced (NEWSIG) but never judged.
	OutcomePending    Outcome = iota
	OutcomeGood               // GOODSIG
	OutcomeBad                // BADSIG
	OutcomeUnknownKey         // ERRSIG: usually the public key is missing
	OutcomeExpiredKey         // EXPKEYSIG
	OutcomeExpiredSig         // EXPSIG
	OutcomeRevokedKey         // REVKEYSIG
)

var outcomeTokens = map[string]Outcome{
	"GOODSIG":   OutcomeGood,
	"BADSIG":    OutcomeBad,
	"ERRSIG":    OutcomeUnknownKey,
	"EXPKEYSIG": OutcomeExpiredKey,
	"EXPSIG":    OutcomeExpiredSig,
	"REVKEYSIG": OutcomeRevokedKey,
}

// Trust is the validity gpg assigned to a signing key (the TRUST_* tokens).
// TrustNone means gpg reported nothing, which is never enough to call a key
// verified.
type Trust int

// Key validity, in increasing order.
const (
	TrustNone Trust = iota
	TrustNever
	TrustUndefined
	TrustMarginal
	TrustFully
	TrustUltimate
)

var trustTokens = map[string]Trust{
	"TRUST_NEVER":     TrustNever,
	"TRUST_UNDEFINED": TrustUndefined,
	"TRUST_MARGINAL":  TrustMarginal,
	"TRUST_FULLY":     TrustFully,
	"TRUST_ULTIMATE":  TrustUltimate,
}

// String names a trust level the way gpg does.
func (t Trust) String() string {
	for token, level := range trustTokens {
		if level == t {
			return strings.ToLower(strings.TrimPrefix(token, "TRUST_"))
		}
	}
	return "unknown"
}

// Signature is everything gpg said about one signature. A message may carry
// several, and each is judged on its own: folding them into one set of fields
// let a good signature's GOODSIG stand beside a forged one's BADSIG.
type Signature struct {
	Outcome Outcome
	KeyID   string
	UserID  string
	// Fingerprint and PrimaryFingerprint come from VALIDSIG. A GOODSIG with
	// no VALIDSIG for the same key is not trusted.
	Fingerprint        string
	PrimaryFingerprint string
	Trust              Trust
	// UIDs are the addresses on the signing key, looked up after the check so
	// the From header can be compared with something the signer controls.
	UIDs []string
}

// validated reports whether a VALIDSIG named the key the GOODSIG did.
func (sig Signature) validated() bool {
	keyID := strings.ToUpper(sig.KeyID)
	if keyID == "" || sig.Fingerprint == "" {
		return false
	}
	return strings.HasSuffix(strings.ToUpper(sig.Fingerprint), keyID) ||
		strings.HasSuffix(strings.ToUpper(sig.PrimaryFingerprint), keyID)
}

// Status is what gpg reported, reduced to the tokens this program acts on.
//
// It is deliberately not a verdict: a signature can be neither good nor bad
// when the signer's public key is missing, which is why OutcomeUnknownKey and
// NoData exist as separate answers. A caller that renders "invalid signature"
// for anything that is not good is lying about mail whose only fault is an
// unknown key.
type Status struct {
	Signatures    []Signature
	NoData        bool
	Decrypted     bool
	DecryptFailed bool
}

// Signed reports whether gpg saw any signature at all.
func (s Status) Signed() bool { return len(s.Signatures) > 0 }

// parseStatus reads the status channel. Unknown tokens are ignored: gpg adds
// them over time, and refusing to work with a newer gpg because it said
// something new would be a strange kind of compatibility.
func parseStatus(text string) Status {
	var s Status
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if !strings.HasPrefix(line, statusPrefix) {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, statusPrefix))
		if len(fields) == 0 {
			continue
		}
		s.apply(fields)
	}
	return s
}

func (s *Status) apply(fields []string) {
	token := fields[0]
	if outcome, ok := outcomeTokens[token]; ok {
		s.judge(outcome, fields)
		return
	}
	if trust, ok := trustTokens[token]; ok {
		s.current().Trust = trust
		return
	}
	switch token {
	case "NEWSIG":
		s.Signatures = append(s.Signatures, Signature{})
	case "VALIDSIG":
		sig := s.current()
		sig.Fingerprint = arg(fields, 1)
		sig.PrimaryFingerprint = arg(fields, 10)
	case "NODATA":
		s.NoData = true
	case "DECRYPTION_OKAY":
		s.Decrypted = true
	case "DECRYPTION_FAILED":
		s.DecryptFailed = true
	}
}

// judge records a signature's outcome. A second outcome without a NEWSIG
// between them (older gpg) starts a new signature rather than overwriting the
// first.
func (s *Status) judge(outcome Outcome, fields []string) {
	sig := s.current()
	if sig.Outcome != OutcomePending {
		s.Signatures = append(s.Signatures, Signature{})
		sig = s.current()
	}
	sig.Outcome = outcome
	sig.KeyID = arg(fields, 1)
	if outcome != OutcomeUnknownKey {
		sig.UserID = signerName(fields, 2)
	}
}

// current is the signature the next VALIDSIG or TRUST_* token belongs to.
func (s *Status) current() *Signature {
	if len(s.Signatures) == 0 {
		s.Signatures = append(s.Signatures, Signature{})
	}
	return &s.Signatures[len(s.Signatures)-1]
}

// signerName rejoins the user id, which gpg writes as the remaining fields.
func signerName(fields []string, from int) string {
	if len(fields) <= from {
		return ""
	}
	return strings.Join(fields[from:], " ")
}

func arg(fields []string, i int) string {
	if i < len(fields) {
		return fields[i]
	}
	return ""
}
