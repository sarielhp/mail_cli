package email

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"path/filepath"
	"strings"
)

// Attachment is one decoded MIME attachment: its sanitized filename, media type
// and content.
type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

// ExtractAttachments returns the attachments of a raw message with their bodies
// decoded; a part that cannot be decoded is skipped, but a failure to parse the
// message or to decode the top-level part is returned as an error.
func ExtractAttachments(data []byte) ([]Attachment, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return extractAttachmentsFromMsg(msg.Header, msg.Body, 0)
}

// SanitizeFilename reduces an attachment filename to a bare, terminal-safe
// basename.
//
// The name arrives from the untrusted Content-Disposition "filename" parameter
// and is joined with a local directory by every save and open path. filepath.Join
// cleans but does not confine, and Go clamps excess ".." at the filesystem root,
// so a name like "../../../../../../.ssh/authorized_keys" escapes the target
// directory without the sender needing to know its depth. Real attachment names
// never contain a path separator.
//
// It also strips terminal escape sequences and control characters: the
// filename is shown verbatim in TUI attachment lists and badges, so a name
// like "evil\x1b[2Jname.txt" could manipulate the terminal display purely
// through its own bytes, independent of the path-traversal confinement
// above.
func SanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(filepath.Clean("/" + name))
	name = StripEscapeSequences(name)
	switch name {
	case "", ".", "..", "/":
		return "attachment"
	}
	return name
}

func resolveAttachmentFilename(header mail.Header, mediaType string, params map[string]string) string {
	filename := ""
	if cdHeader := header.Get("Content-Disposition"); cdHeader != "" {
		if cdType, cdParams, err := mime.ParseMediaType(cdHeader); err == nil {
			if cdType == "attachment" || cdType == "inline" {
				filename = cdParams["filename"]
			}
		}
	}
	if filename == "" && params != nil {
		filename = params["name"]
	}
	if filename == "" && (mediaType == "text/calendar" || mediaType == "application/ics" || mediaType == "text/x-vcalendar") {
		filename = "invite.ics"
	}
	if filename == "" {
		return ""
	}
	dec := new(mime.WordDecoder)
	if decoded, err := dec.DecodeHeader(filename); err == nil {
		filename = decoded
	}
	return SanitizeFilename(filename)
}

// decodeAttachmentData decodes an attachment part's body per its
// Content-Transfer-Encoding.
//
// It used to fall back to returning the raw, still-encoded bytes with a nil
// error when every decode attempt failed, so a corrupted attachment looked
// like it had decoded successfully -- its Data was the undecoded base64 or
// quoted-printable text, not real content, and saving or opening it produced
// garbage with no indication anything had gone wrong. Returning the error
// instead lets the existing callers do what they already do for any other
// decode failure: extractMultipartAttachments skips the one bad part and
// keeps the rest (extractAttachmentsFromMsg's `if err == nil` in its loop),
// rather than presenting corrupted content as if it were real.
func decodeAttachmentData(body io.Reader, cte string) ([]byte, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	switch cte {
	case "base64":
		decoder := base64.NewDecoder(base64.StdEncoding, bytes.NewReader(data))
		if decodedData, err := io.ReadAll(decoder); err == nil {
			return decodedData, nil
		}
		lenient := base64.NewDecoder(base64.RawStdEncoding, bytes.NewReader(bytes.ReplaceAll(data, []byte("\r\n"), []byte(""))))
		lData, lErr := io.ReadAll(lenient)
		if lErr == nil {
			return lData, nil
		}
		return nil, fmt.Errorf("decoding base64 attachment: %w", lErr)
	case "quoted-printable":
		decoder := quotedprintable.NewReader(bytes.NewReader(data))
		decodedData, qpErr := io.ReadAll(decoder)
		if qpErr == nil {
			return decodedData, nil
		}
		return nil, fmt.Errorf("decoding quoted-printable attachment: %w", qpErr)
	default:
		return data, nil
	}
}

// The attachment walks honour the same maxMIMEDepth as the body walk in
// body.go, which already had a cap these did not. Real mail is two to four
// levels deep; this runs automatically whenever a message is opened or scanned,
// on input the sender chooses, and without a limit a message nesting
// multipart/mixed deeply enough is a stack overflow -- which in Go is a fatal
// error no recover() can catch.

func extractMultipartAttachments(body io.Reader, boundary string, depth int) []Attachment {
	if boundary == "" || depth >= maxMIMEDepth {
		return nil
	}
	var attachments []Attachment
	mr := multipart.NewReader(body, boundary)
	for {
		part, err := mr.NextPart()
		if err != nil {
			break
		}
		partHeader := mail.Header(part.Header)
		partAtts, err := extractAttachmentsFromMsg(partHeader, part, depth+1)
		if err == nil {
			attachments = append(attachments, partAtts...)
		}
	}
	return attachments
}

func extractAttachmentsFromMsg(header mail.Header, body io.Reader, depth int) ([]Attachment, error) {
	mediaType, params, _ := mime.ParseMediaType(header.Get("Content-Type"))
	filename := resolveAttachmentFilename(header, mediaType, params)
	if filename != "" {
		cte := strings.ToLower(header.Get("Content-Transfer-Encoding"))
		data, err := decodeAttachmentData(body, cte)
		if err != nil {
			return nil, err
		}
		return []Attachment{{
			Filename:    filename,
			ContentType: mediaType,
			Data:        data,
		}}, nil
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		return extractMultipartAttachments(body, params["boundary"], depth), nil
	}
	return nil, nil
}

func isICSMediaTypeOrName(mediaType, name, filename string) bool {
	if mediaType == "text/calendar" || mediaType == "application/ics" || mediaType == "text/x-vcalendar" {
		return true
	}
	if name != "" && strings.HasSuffix(strings.ToLower(name), ".ics") {
		return true
	}
	if filename != "" && strings.HasSuffix(strings.ToLower(filename), ".ics") {
		return true
	}
	return false
}

// IsICSAttachment returns true if the attachment is an ICS / calendar file.
func IsICSAttachment(att Attachment) bool {
	return isICSMediaTypeOrName(att.ContentType, "", att.Filename)
}

// DetectAttachments traverses MIME headers and body in a single pass to determine
// whether the message contains non-ICS attachments and/or ICS calendar events.
func DetectAttachments(header mail.Header, body io.Reader) (hasAttachment bool, hasICS bool) {
	return detectAttachments(header, body, 0)
}

func detectAttachments(header mail.Header, body io.Reader, depth int) (hasAttachment bool, hasICS bool) {
	mediaType, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil {
		ct := strings.ToLower(header.Get("Content-Type"))
		if strings.Contains(ct, "text/calendar") || strings.Contains(ct, "application/ics") || strings.Contains(ct, ".ics") {
			return false, true
		}
		return false, false
	}

	name := params["name"]
	filename := ""
	isAttachmentDisposition := false
	cdHeader := header.Get("Content-Disposition")
	if cdHeader != "" {
		if cdType, cdParams, errCD := mime.ParseMediaType(cdHeader); errCD == nil {
			if cdType == "attachment" {
				isAttachmentDisposition = true
			}
			filename = cdParams["filename"]
		}
	}

	if isICSMediaTypeOrName(mediaType, name, filename) {
		hasICS = true
	} else if isAttachmentDisposition || ((name != "" || filename != "") && !strings.HasPrefix(mediaType, "multipart/")) {
		hasAttachment = true
	}

	if strings.HasPrefix(mediaType, "multipart/") && depth < maxMIMEDepth {
		boundary := params["boundary"]
		if boundary != "" {
			mr := multipart.NewReader(body, boundary)
			for {
				part, errPart := mr.NextPart()
				if errPart == io.EOF || errPart != nil {
					break
				}
				partHeader := mail.Header(part.Header)
				subAtt, subICS := detectAttachments(partHeader, part, depth+1)
				if subAtt {
					hasAttachment = true
				}
				if subICS {
					hasICS = true
				}
			}
		}
	}

	return hasAttachment, hasICS
}

func hasICSAttachmentInMsg(header mail.Header, body io.Reader) bool {
	_, hasICS := DetectAttachments(header, body)
	return hasICS
}

func hasAttachmentInMsg(header mail.Header, body io.Reader) bool {
	hasAtt, _ := DetectAttachments(header, body)
	return hasAtt
}
