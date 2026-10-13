package email

import (
	"encoding/base64"
	"net/url"
	"regexp"
	"strings"
)

var (
	urlDefenseV1V2Regex = regexp.MustCompile(`https?://urldefense(?:\.proofpoint)?\.com/(v[12])/url\?([^\s"'>)]+)`)
	urlRegex            = regexp.MustCompile(`https?://urldefense(?:\.proofpoint)?\.com/(?:v[123])/[^\s"'>)]+`)
)

var v3Mapping = func() map[byte]int {
	m := make(map[byte]int)
	chars := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	for i, c := range chars {
		m[byte(c)] = i + 2
	}
	return m
}()

// DecodeURLDefense decodes a Proofpoint URLDefense (v1, v2, or v3) rewritten URL
// into its original destination URL. If urlStr is not a recognized URLDefense URL,
// it returns (urlStr, false).
func DecodeURLDefense(urlStr string) (string, bool) {
	if !strings.Contains(urlStr, "urldefense") {
		return urlStr, false
	}

	if strings.Contains(urlStr, "/v3/__") {
		return decodeURLDefenseV3(urlStr)
	}

	if m := urlDefenseV1V2Regex.FindStringSubmatch(urlStr); len(m) > 2 {
		version := m[1]
		queryStr := m[2]
		vals, err := url.ParseQuery(queryStr)
		if err != nil {
			return urlStr, false
		}
		uParam := vals.Get("u")
		if uParam == "" {
			return urlStr, false
		}
		if version == "v1" {
			return decodeURLDefenseV1(uParam)
		}
		return decodeURLDefenseV2(uParam)
	}

	return urlStr, false
}

func decodeURLDefenseV1(u string) (string, bool) {
	unescaped, err := url.QueryUnescape(u)
	if err != nil {
		return u, false
	}
	return unescaped, true
}

func decodeURLDefenseV2(u string) (string, bool) {
	var sb strings.Builder
	runes := []rune(u)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '-' {
			if i+2 < len(runes) && isHex(runes[i+1]) && isHex(runes[i+2]) {
				sb.WriteRune('%')
				sb.WriteRune(runes[i+1])
				sb.WriteRune(runes[i+2])
				i += 2
				continue
			}
			if i+1 < len(runes) && runes[i+1] == '-' {
				sb.WriteRune('-')
				i++
				continue
			}
			sb.WriteRune('%')
			continue
		}
		if r == '_' {
			sb.WriteRune('/')
			continue
		}
		sb.WriteRune(r)
	}

	unescaped, err := url.QueryUnescape(sb.String())
	if err != nil {
		return sb.String(), true
	}
	return unescaped, true
}

func isHex(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func decodeURLDefenseV3(rawURL string) (string, bool) {
	startIdx := strings.Index(rawURL, "/v3/__")
	if startIdx == -1 {
		return rawURL, false
	}
	urlPart := rawURL[startIdx+len("/v3/__"):]
	endIdx := strings.Index(urlPart, "__;")
	if endIdx == -1 {
		return rawURL, false
	}

	encodedURL := urlPart[:endIdx]
	remainder := urlPart[endIdx+len("__;"):]

	exclIdx := strings.Index(remainder, "!")
	var replB64 string
	if exclIdx == -1 {
		replB64 = remainder
	} else {
		replB64 = remainder[:exclIdx]
	}

	if replB64 == "" {
		return encodedURL, true
	}

	pad := len(replB64) % 4
	if pad > 0 {
		replB64 += strings.Repeat("=", 4-pad)
	}
	replBytes, err := base64.URLEncoding.DecodeString(replB64)
	if err != nil {
		replBytes, err = base64.StdEncoding.DecodeString(replB64)
		if err != nil {
			return encodedURL, true
		}
	}

	if len(replBytes) == 0 {
		return encodedURL, true
	}

	return applyV3Replacements(encodedURL, replBytes)
}

func applyV3Replacements(encodedURL string, replBytes []byte) (string, bool) {
	var out strings.Builder
	replIdx := 0
	b := []byte(encodedURL)

	for i := 0; i < len(b); i++ {
		if b[i] == '*' {
			if i+2 < len(b) && b[i+1] == '*' {
				x := b[i+2]
				numBytes, ok := v3Mapping[x]
				if ok && replIdx+numBytes <= len(replBytes) {
					out.Write(replBytes[replIdx : replIdx+numBytes])
					replIdx += numBytes
					i += 2
					continue
				}
			}
			if replIdx < len(replBytes) {
				out.WriteByte(replBytes[replIdx])
				replIdx++
				continue
			}
		}
		out.WriteByte(b[i])
	}

	return out.String(), true
}

// UnwrapURLDefenseInText replaces all Proofpoint URLDefense URLs in a text with their decoded equivalents.
func UnwrapURLDefenseInText(text string) string {
	if !strings.Contains(text, "urldefense") {
		return text
	}
	return urlRegex.ReplaceAllStringFunc(text, func(m string) string {
		clean, ok := DecodeURLDefense(m)
		if ok {
			return clean
		}
		return m
	})
}
