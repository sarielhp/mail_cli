package pgp

import (
	"bytes"
	"context"
	"net/mail"
	"strconv"
	"strings"

	"github.com/sarielhp/mail_cli/email"
)

// failedOutcome is how a signature that is not good is reported. The rank
// orders them: when a message carries several signatures, the worst decides.
type failedOutcome struct {
	rank    int
	verdict string
	message string
}

var failedOutcomes = map[Outcome]failedOutcome{
	OutcomeBad:        {6, email.VerdictInvalid, "signature verification failed"},
	OutcomeRevokedKey: {5, email.VerdictRevoked, "the signing key has been revoked"},
	OutcomeExpiredKey: {4, email.VerdictExpired, "the signing key has expired"},
	OutcomeExpiredSig: {4, email.VerdictExpired, "the signature has expired"},
	OutcomeUnknownKey: {3, email.VerdictUnknownKey, "no public key for the signature"},
	OutcomePending:    {2, email.VerdictError, "gpg did not report a result for every signature"},
}

// judgement is the verdict on a set of signatures and the one that decided it.
type judgement struct {
	verdict string
	message string
	sig     Signature
}

// judgeSignatures reduces every signature gpg reported to one verdict.
//
// Any signature that is not good makes the whole message not good: a valid
// signature beside a forged one used to leave the message "valid". Among good
// ones, a GOODSIG without a VALIDSIG naming the same key is not believed, and
// a key below full trust gives "good signature, unverified key".
func judgeSignatures(sigs []Signature) judgement {
	worst, worstRank := -1, 0
	for i, sig := range sigs {
		if failed, ok := failedOutcomes[sig.Outcome]; ok && failed.rank > worstRank {
			worst, worstRank = i, failed.rank
		}
	}
	if worst >= 0 {
		failed := failedOutcomes[sigs[worst].Outcome]
		return judgement{verdict: failed.verdict, message: failed.message, sig: sigs[worst]}
	}
	weakest := 0
	for i, sig := range sigs {
		if !sig.validated() {
			return judgement{verdict: email.VerdictError,
				message: "gpg reported a good signature without validating it", sig: sig}
		}
		if sig.Trust < sigs[weakest].Trust {
			weakest = i
		}
	}
	if sigs[weakest].Trust < TrustFully {
		return judgement{verdict: email.VerdictUnverifiedKey,
			message: "the signing key is not verified (trust: " + sigs[weakest].Trust.String() + ")", sig: sigs[weakest]}
	}
	return judgement{verdict: email.VerdictValid, sig: sigs[weakest]}
}

// signedBy reports whether any good signature's key carries address.
func signedBy(sigs []Signature, address string) bool {
	address = strings.ToLower(strings.TrimSpace(address))
	if address == "" {
		return false
	}
	for _, sig := range sigs {
		if sig.Outcome != OutcomeGood {
			continue
		}
		uids := sig.UIDs
		if len(uids) == 0 {
			uids = []string{sig.UserID}
		}
		for _, uid := range uids {
			if strings.ToLower(uidAddress(uid)) == address {
				return true
			}
		}
	}
	return false
}

// uidAddress is the address in a user id ("Name <addr>"), or the user id
// itself when it is a bare address.
func uidAddress(uid string) string {
	if addr, err := mail.ParseAddress(uid); err == nil {
		return addr.Address
	}
	open, closing := strings.LastIndexByte(uid, '<'), strings.LastIndexByte(uid, '>')
	if open >= 0 && closing > open {
		return strings.TrimSpace(uid[open+1 : closing])
	}
	return strings.TrimSpace(uid)
}

// senderAddress is the bare address in a message's From header, or "" when
// there is none that parses.
func senderAddress(raw []byte) string {
	header, _ := email.SplitHeaderBody(raw)
	msg, err := mail.ReadMessage(bytes.NewReader(append(append([]byte{}, header...), "\r\n\r\n"...)))
	if err != nil {
		return ""
	}
	addr, err := mail.ParseAddress(msg.Header.Get("From"))
	if err != nil {
		return ""
	}
	return addr.Address
}

// withSignerUIDs records the user ids on the key behind each good signature,
// so the From address is compared against every identity the key holds rather
// than only the primary one gpg names in GOODSIG.
func withSignerUIDs(ctx context.Context, s Status) Status {
	for i := range s.Signatures {
		sig := &s.Signatures[i]
		if sig.Outcome != OutcomeGood || !sig.validated() {
			continue
		}
		key := sig.PrimaryFingerprint
		if key == "" {
			key = sig.Fingerprint
		}
		sig.UIDs = keyUIDs(ctx, key)
	}
	return s
}

// keyUIDs lists the user ids on a key that are not revoked.
func keyUIDs(ctx context.Context, fingerprint string) []string {
	res, err := execGPG(ctx, nil, "--with-colons", "--fixed-list-mode", "--list-keys", "--", fingerprint)
	if err != nil || res.ExitErr != nil || res.LimitErr != nil {
		return nil
	}
	var uids []string
	for _, line := range strings.Split(string(res.Stdout), "\n") {
		fields := strings.Split(strings.TrimRight(line, "\r"), ":")
		if len(fields) < 10 || fields[0] != "uid" || fields[1] == "r" {
			continue
		}
		uids = append(uids, unescapeColons(fields[9]))
	}
	return uids
}

// unescapeColons undoes gpg's \xNN escaping of a colon-listing field.
func unescapeColons(field string) string {
	if !strings.Contains(field, `\x`) {
		return field
	}
	var b strings.Builder
	for i := 0; i < len(field); i++ {
		if field[i] == '\\' && i+3 < len(field) && field[i+1] == 'x' {
			if v, err := strconv.ParseUint(field[i+2:i+4], 16, 8); err == nil {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(field[i])
	}
	return b.String()
}
