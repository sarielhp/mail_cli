package email

import (
	"bytes"
	"fmt"
	"mime"
	"net/mail"
	"strings"
)

// ReplyDetails contains pre-computed headers and quoted body for replying to an email.
type ReplyDetails struct {
	To         string
	Cc         []string
	Subject    string
	InReplyTo  string
	References string
	QuotedBody string
}

// BuildReplyDetails extracts reply headers and quotes the original message.
func BuildReplyDetails(orig *mail.Message, ourEmail string, groupReply bool) ReplyDetails {
	if orig == nil {
		return ReplyDetails{}
	}

	toAddress := orig.Header.Get("Reply-To")
	if toAddress == "" {
		toAddress = orig.Header.Get("From")
	}
	toAddress = strings.TrimSpace(toAddress)

	dec := new(mime.WordDecoder)
	subject := DecDecode(dec, orig.Header.Get("Subject"))
	subject = formatReplySubject(subject)

	msgID := strings.TrimSpace(orig.Header.Get("Message-ID"))
	origRefs := strings.TrimSpace(orig.Header.Get("References"))
	references := msgID
	if origRefs != "" {
		references = origRefs + " " + msgID
	}

	var cc []string
	if groupReply {
		cc = BuildGroupReplyCC(orig, toAddress, ourEmail)
	}

	return ReplyDetails{
		To:         toAddress,
		Cc:         cc,
		Subject:    subject,
		InReplyTo:  msgID,
		References: references,
		QuotedBody: FormatReplyQuote(orig),
	}
}

// formatReplySubject ensures the subject starts with "Re: ".
func formatReplySubject(subj string) string {
	trimmed := strings.TrimSpace(subj)
	if trimmed == "" {
		return "Re: "
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "re:") {
		return trimmed
	}
	return "Re: " + trimmed
}

// FormatReplyQuote formats the plain-text body of the original message into a standard quote.
func FormatReplyQuote(orig *mail.Message) string {
	bodyText, err := ExtractPlainBodyText(orig)
	if err != nil || strings.TrimSpace(bodyText) == "" {
		return ""
	}

	bodyText = strings.ReplaceAll(bodyText, "\r\n", "\n")
	bodyText = strings.ReplaceAll(bodyText, "\r", "\n")

	dec := new(mime.WordDecoder)
	origFrom := DecDecode(dec, orig.Header.Get("From"))
	origDate := orig.Header.Get("Date")

	var buf bytes.Buffer
	switch {
	case origDate != "" && origFrom != "":
		fmt.Fprintf(&buf, "On %s, %s wrote:\n", origDate, origFrom)
	case origFrom != "":
		fmt.Fprintf(&buf, "%s wrote:\n", origFrom)
	default:
		buf.WriteString("Quoted message:\n")
	}

	lines := strings.Split(strings.TrimSpace(bodyText), "\n")
	for _, line := range lines {
		fmt.Fprintf(&buf, "> %s\n", line)
	}

	return buf.String()
}

// BuildGroupReplyCC extracts all recipient addresses from To and Cc, excluding ourEmail and toAddress.
func BuildGroupReplyCC(origMsg *mail.Message, toAddress string, ourEmail string) []string {
	seen := make(map[string]bool)
	if ourEmail != "" {
		seen[strings.ToLower(strings.TrimSpace(ourEmail))] = true
	}

	if primaryTo, errTo := mail.ParseAddress(toAddress); errTo == nil && primaryTo != nil {
		seen[strings.ToLower(primaryTo.Address)] = true
	} else if toAddress != "" {
		seen[strings.ToLower(strings.TrimSpace(toAddress))] = true
	}

	var ccAddresses []string
	for _, hdrVal := range []string{origMsg.Header.Get("To"), origMsg.Header.Get("Cc")} {
		if hdrVal == "" {
			continue
		}
		addrs, err := mail.ParseAddressList(hdrVal)
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			emailLower := strings.ToLower(addr.Address)
			if !seen[emailLower] {
				seen[emailLower] = true
				ccAddresses = append(ccAddresses, addr.String())
			}
		}
	}
	return ccAddresses
}
