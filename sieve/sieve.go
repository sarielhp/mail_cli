package sieve

import (
	"fmt"
	"os"
	"strings"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/report"
)

// ExportScript writes a Sieve script generated from the resolved account's rules
// to outputPath and reports the result; it errors when the account has no rules
// or the file cannot be written.
func ExportScript(rep report.Reporter, config *cfg_g.Config, outputPath string) error {
	resolved, err := cfg_g.Resolve(config)
	if err != nil {
		return err
	}

	rules := resolved.Account.Rules
	if len(rules) == 0 {
		return fmt.Errorf("no rules defined for account %s", resolved.Account.Name)
	}

	sieveScript := generateSieveScript(rules)

	err = os.WriteFile(outputPath, []byte(sieveScript), 0600)
	if err != nil {
		return fmt.Errorf("failed to write Sieve script: %w", err)
	}

	rep.Successf("Generated Sieve script for account %s with %d rule(s)\n", resolved.Account.Name, len(rules))
	rep.Infof("Sieve script written to: %s\n", outputPath)
	rep.Warnf("Note: FastMail does not support JMAP server-side filters.\n")
	rep.Warnf("Upload this script to FastMail at: https://www.fastmail.com/help/objects/filters\n")
	rep.Warnf("(Settings > Filters > Manage Sieve Scripts)\n\n")

	rep.Printf("=== Sieve script content ===\n")
	rep.Printf("%v", sieveScript)

	return nil
}

func generateSieveScript(rules []cfg_acc.Rule) string {
	var sb strings.Builder

	sb.WriteString("# mail_cli generated Sieve script\n")
	sb.WriteString("# Rules exported from local configuration\n")
	sb.WriteString("# Upload this to FastMail via: Settings > Filters > Manage Sieve Scripts\n")
	sb.WriteString("#\n")
	sb.WriteString("# See: https://www.fastmail.com/help/objects/filters.html\n\n")

	sb.WriteString("require [\"fileinto\"];\n\n")

	for i, rule := range rules {
		if rule.Sender == "" && rule.Subject == "" {
			continue
		}

		label := sanitizeForSieve(rule.Label)

		if rule.Subject != "" {
			subjPattern := sanitizeForSieve(rule.Subject) + "*"
			sb.WriteString(fmt.Sprintf("# Rule %d: subject:%s -> %s\n", i+1, sanitizeForSieveComment(rule.Subject), sanitizeForSieveComment(rule.Label)))
			sb.WriteString(fmt.Sprintf("if header :matches \"Subject\" \"%s\"\n", subjPattern))
		} else {
			sender := sanitizeForSieve(rule.Sender)
			sb.WriteString(fmt.Sprintf("# Rule %d: %s -> %s\n", i+1, sanitizeForSieveComment(rule.Sender), sanitizeForSieveComment(rule.Label)))
			if strings.HasPrefix(sender, "@") || strings.HasPrefix(sender, "*@") {
				pattern := "*" + strings.TrimPrefix(sender, "*")
				sb.WriteString(fmt.Sprintf("if address :matches \"from\" \"%s\"\n", pattern))
			} else {
				sb.WriteString(fmt.Sprintf("if address :is \"from\" \"%s\"\n", sender))
			}
		}
		sb.WriteString("{\n")
		sb.WriteString(fmt.Sprintf("    fileinto \"%s\";\n", label))
		sb.WriteString("    stop;\n")
		sb.WriteString("}\n\n")
	}

	return sb.String()
}

// sanitizeForSieve makes a value safe inside a Sieve quoted string.
//
// Backslash first, then quote -- the other order double-escapes. Control
// characters are dropped rather than escaped: a Sieve quoted string cannot
// contain a raw newline at all, so a rule whose sender or label held one used
// to end the string mid-literal and leave the remainder of the line as script
// text. Rule values are not typed by hand only; they are derived from message
// senders and subjects, and config.json can carry a newline inside a JSON
// string perfectly legally.
func sanitizeForSieve(s string) string {
	s = stripControl(s)
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// sanitizeForSieveComment makes a value safe on a comment line, where a newline
// ends the comment and anything after it is script.
func sanitizeForSieveComment(s string) string {
	return stripControl(s)
}

// stripControl removes newlines, carriage returns, tabs and other C0/C7
// control characters, replacing a run of them with a single space.
func stripControl(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return r < 0x20 || r == 0x7f
	}), " ")
}
