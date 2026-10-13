package scan

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/email"
)

func TestIsRuneAllowed(t *testing.T) {
	tests := []struct {
		name      string
		r         rune
		whitelist []string
		want      bool
	}{
		{"space is allowed", ' ', []string{"english"}, true},
		{"digit is allowed", '1', []string{"english"}, true},
		{"latin letter allowed", 'a', []string{"english"}, true},
		{"latin letter with accent allowed", 'é', []string{"french"}, true},
		{"cyrillic letter not allowed for english", 'ж', []string{"english"}, false},
		{"hebrew letter allowed for hebrew", 'א', []string{"hebrew"}, true},
		{"hebrew letter not allowed for english", 'א', []string{"english"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRuneAllowed(tt.r, tt.whitelist)
			if got != tt.want {
				t.Errorf("isRuneAllowed(%q, %v) = %v, want %v", tt.r, tt.whitelist, got, tt.want)
			}
		})
	}
}

// CleanTextForNLP keeps letters and whitespace, drops punctuation, digits and
// URLs, and is stable: cleaning text that is already clean changes nothing.
func TestCleanTextForNLP(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"letters and spaces preserved", "hello world", "hello world"},
		{"punctuation removed", "hello, world!", "hello world"},
		{"numbers removed", "hello123world", "helloworld"},
		{"unicode letters preserved", "こんにちは world", "こんにちは world"},
		{"urls removed", "hello https://urldefense.com/v3/__https://www.google.com/__;!!abc!def$ world", "hello  world"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := cleanTextForNLP(tt.input)
			if got != tt.want {
				t.Errorf("cleanTextForNLP(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if again := cleanTextForNLP(got); again != got {
				t.Errorf("cleanTextForNLP is not idempotent: clean(%q) = %q, clean(%q) = %q",
					tt.input, got, got, again)
			}
		})
	}
}

func TestIsDetectedLanguageWhitelisted(t *testing.T) {
	tests := []struct {
		name      string
		detected  string
		iso6391   string
		whitelist []string
		want      bool
	}{
		{"english matches english", "english", "en", []string{"english"}, true},
		{"german matches german", "german", "de", []string{"german"}, true},
		{"french matches french", "french", "fr", []string{"french"}, true},
		{"hebrew matches hebrew", "hebrew", "he", []string{"hebrew"}, true},
		{"hebrew matches iw alias", "hebrew", "iw", []string{"hebrew"}, true},
		{"no match", "russian", "ru", []string{"english"}, false},
		{"empty whitelist", "english", "en", []string{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isDetectedLanguageWhitelisted(tt.detected, tt.iso6391, tt.whitelist)
			if got != tt.want {
				t.Errorf("isDetectedLanguageWhitelisted(%q, %q, %v) = %v, want %v",
					tt.detected, tt.iso6391, tt.whitelist, got, tt.want)
			}
			// The comparison lowercases both sides, so an upper-cased
			// whitelist must produce the same verdict.
			upper := make([]string, len(tt.whitelist))
			for i, lang := range tt.whitelist {
				upper[i] = strings.ToUpper(lang)
			}
			if upperGot := isDetectedLanguageWhitelisted(tt.detected, tt.iso6391, upper); upperGot != got {
				t.Errorf("whitelist matching is case-sensitive: %v gave %v but %v gave %v",
					tt.whitelist, got, upper, upperGot)
			}
		})
	}
}

func TestDetectPolitical(t *testing.T) {
	tests := []struct {
		name     string
		subject  string
		body     string
		wantBool bool
	}{
		{"actblue link is political", "Support Us", "Click here: https://actblue.com/donate", true},
		{"winred link is political", "Donate Today", "Visit winred.com to contribute", true},
		{"paid for by is political", "Campaign Update", "Paid for by Friends of Candidate", true},
		{"no political content", "Hello World", "This is a normal email", false},
		{"tax deductible disclosure", "Update", "Contributions are not tax deductible", true},
		{"not authorized by candidate", "Notice", "Not authorized by any candidate", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBool, _, _ := email.DetectPolitical(tt.subject, tt.body)
			if gotBool != tt.wantBool {
				t.Errorf("email.DetectPolitical(%q, %q) = %v, want %v",
					tt.subject, tt.body, gotBool, tt.wantBool)
			}
		})
	}
}

func TestDetectScriptLabel(t *testing.T) {
	tests := []struct {
		name  string
		r     rune
		label string
	}{
		{"cyrillic", 'ж', "Cyrillic/Russian"},
		{"greek", 'ω', "Greek"},
		{"han", '汉', "Han"},
		{"hiragana", 'あ', "Japanese"},
		{"katakana", 'ア', "Japanese"},
		{"hangul", '한', "Korean"},
		{"arabic", 'ع', "Arabic"},
		{"devanagari", 'अ', "Hindi/Devanagari"},
		{"thai", 'ก', "Thai"},
		{"unknown", '!', "Unknown Script"},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("%02d %s", i, tt.name), func(t *testing.T) {
			t.Parallel()
			got := detectScriptLabel(tt.r)
			if got != tt.label {
				t.Errorf("detectScriptLabel(%q) = %q, want %q", tt.r, got, tt.label)
			}
		})
	}
}

func TestStripHTML(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "Hello <b>World</b>!",
			expected: "Hello World!",
		},
		{
			input:    "<html><head><title>My Title</title><style>body { color: red; }</style></head><body>Hello World<script>console.log('hi');</script></body></html>",
			expected: "Hello World",
		},
		{
			input:    "Text with &nbsp; entities &amp; tags &lt; &gt;.",
			expected: "Text with entities & tags < >.",
		},
	}

	for _, tt := range tests {
		got := email.StripHTML(tt.input)
		if got != tt.expected {
			t.Errorf("email.StripHTML(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
