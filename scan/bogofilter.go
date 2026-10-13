package scan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ClassifierResult represents the result of a single classifier
type ClassifierResult struct {
	Name        string
	Score       float64
	Description string
	IsSpam      bool
}

// bogofilterTimeout bounds a single classification. Without it, a hung
// bogofilter (for example on a corrupted wordlist) blocked its goroutine
// forever, and because Emails waits on every goroutine the whole scan hung
// with no diagnostic.
const bogofilterTimeout = 30 * time.Second

// classifyWithBogofilter runs Bogofilter on an email.
//
// It returns an error when bogofilter could not be run or did not produce a
// usable verdict. The error used to be discarded with `_ = cmd.Run()`, and the
// zero values then read as a confident "Ham" with score 0.0 -- so a missing or
// broken binary silently disabled spam classification entirely while every scan
// reported success.
func classifyWithBogofilter(emailBytes []byte) (ClassifierResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), bogofilterTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bogofilter", "-v")
	cmd.Stdin = bytes.NewReader(emailBytes)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	// bogofilter uses exit status to convey its verdict (0 spam, 1 ham,
	// 2 unsure), so a non-zero status is not itself a failure. Distinguish
	// "could not run" and "timed out" from a normal verdict.
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return ClassifierResult{Name: "BOGOFILTER"}, fmt.Errorf("bogofilter timed out after %s", bogofilterTimeout)
	}
	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			return ClassifierResult{Name: "BOGOFILTER"}, fmt.Errorf("running bogofilter: %w (stderr: %s)", runErr, strings.TrimSpace(errBuf.String()))
		}
	}

	output := outBuf.String()
	if strings.TrimSpace(output) == "" {
		return ClassifierResult{Name: "BOGOFILTER"}, fmt.Errorf("bogofilter produced no output (stderr: %s)", strings.TrimSpace(errBuf.String()))
	}
	classification := "Ham"
	spamicity := 0.0

	if contains(output, "Spam") {
		classification = "Spam"
	} else if contains(output, "Unsure") {
		classification = "Unsure"
	}

	if idx := indexOf(output, "spamicity="); idx != -1 {
		sub := output[idx+10:]
		if endIdx := indexOfAny(sub, ", \r\n"); endIdx != -1 {
			sub = sub[:endIdx]
		}
		if val, err := strconv.ParseFloat(strings.TrimSpace(sub), 64); err == nil {
			spamicity = val
		}
	}

	isSpam := spamicity >= 0.5 || classification == "Spam"

	return ClassifierResult{
		Name:        "BOGOFILTER",
		Score:       spamicity,
		Description: "Bogofilter classification: " + classification + " (spamicity: " + strconv.FormatFloat(spamicity, 'f', -1, 64) + ")",
		IsSpam:      isSpam,
	}, nil
}

func contains(s, substr string) bool {
	return indexOf(s, substr) != -1
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func indexOfAny(s string, chars string) int {
	for i, r := range s {
		if strings.ContainsRune(chars, r) {
			return i
		}
	}
	return -1
}
