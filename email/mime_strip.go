package email

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"net/textproto"
	"strings"
)

// StripAttachmentsFromEML rewrites a multipart email payload, replacing the MIME parts
// matching any of targetFilenames with a text marker: [Attachment deleted: <filename>].
// Returns the modified EML data, whether any attachment was stripped, and any error.
func StripAttachmentsFromEML(rawEML []byte, targetFilenames []string) ([]byte, bool, error) {
	if len(rawEML) == 0 || len(targetFilenames) == 0 {
		return rawEML, false, nil
	}

	targets := make(map[string]bool, len(targetFilenames))
	for _, fn := range targetFilenames {
		targets[strings.ToLower(SanitizeFilename(fn))] = true
	}

	headerBytes, bodyBytes, sep, err := splitEMLHeaderAndBody(rawEML)
	if err != nil {
		return rawEML, false, err
	}

	msg, err := mail.ReadMessage(bytes.NewReader(rawEML))
	if err != nil {
		return rawEML, false, err
	}

	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		return rawEML, false, nil
	}

	boundary := params["boundary"]
	if boundary == "" {
		return rawEML, false, nil
	}

	newBody, stripped, err := rewriteMultipart(bodyBytes, boundary, targets, 0)
	if err != nil || !stripped {
		return rawEML, stripped, err
	}

	var result bytes.Buffer
	result.Write(headerBytes)
	result.Write(sep)
	result.Write(newBody)
	return result.Bytes(), true, nil
}

func splitEMLHeaderAndBody(raw []byte) ([]byte, []byte, []byte, error) {
	idx := bytes.Index(raw, []byte("\r\n\r\n"))
	if idx != -1 {
		return raw[:idx], raw[idx+4:], []byte("\r\n\r\n"), nil
	}
	idx = bytes.Index(raw, []byte("\n\n"))
	if idx != -1 {
		return raw[:idx], raw[idx+2:], []byte("\n\n"), nil
	}
	return nil, nil, nil, fmt.Errorf("malformed EML: no header/body separator found")
}

func rewriteMultipart(bodyBytes []byte, boundary string, targets map[string]bool, depth int) ([]byte, bool, error) {
	if depth >= maxMIMEDepth {
		return bodyBytes, false, nil
	}

	mr := multipart.NewReader(bytes.NewReader(bodyBytes), boundary)
	var outBuf bytes.Buffer
	mw := multipart.NewWriter(&outBuf)
	if err := mw.SetBoundary(boundary); err != nil {
		return bodyBytes, false, err
	}

	anyStripped := false
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return bodyBytes, false, err
		}

		partHeader := mail.Header(part.Header)
		partMediaType, partParams, _ := mime.ParseMediaType(partHeader.Get("Content-Type"))
		partFilename := resolveAttachmentFilename(partHeader, partMediaType, partParams)

		if partFilename != "" && targets[strings.ToLower(partFilename)] {
			anyStripped = true
			pw, createErr := mw.CreatePart(textproto.MIMEHeader{
				"Content-Type":        []string{"text/plain; charset=\"utf-8\""},
				"Content-Disposition": []string{"inline"},
			})
			if createErr != nil {
				return bodyBytes, false, createErr
			}
			_, _ = pw.Write([]byte(fmt.Sprintf("[Attachment deleted: %s]", partFilename)))
			continue
		}

		if strings.HasPrefix(partMediaType, "multipart/") && partParams["boundary"] != "" {
			subBody, readErr := io.ReadAll(part)
			if readErr != nil {
				return bodyBytes, false, readErr
			}
			rewrittenSubBody, subStripped, rewriteErr := rewriteMultipart(subBody, partParams["boundary"], targets, depth+1)
			if rewriteErr == nil && subStripped {
				anyStripped = true
				pw, subCreateErr := mw.CreatePart(part.Header)
				if subCreateErr != nil {
					return bodyBytes, false, subCreateErr
				}
				_, _ = pw.Write(rewrittenSubBody)
				continue
			}
			pw, partCreateErr := mw.CreatePart(part.Header)
			if partCreateErr != nil {
				return bodyBytes, false, partCreateErr
			}
			_, _ = pw.Write(subBody)
			continue
		}

		pw, err := mw.CreatePart(part.Header)
		if err != nil {
			return bodyBytes, false, err
		}
		if _, err := io.Copy(pw, part); err != nil {
			return bodyBytes, false, err
		}
	}

	if err := mw.Close(); err != nil {
		return bodyBytes, false, err
	}

	return outBuf.Bytes(), anyStripped, nil
}
