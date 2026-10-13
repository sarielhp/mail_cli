package spam

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const bogofilterTimeout = 30 * time.Second

// TrainHam registers an email as legitimate (HAM) in the local Bogofilter database.
func TrainHam(emailBytes []byte) error {
	return trainBogofilter(emailBytes, "-n", "ham")
}

// TrainSpam registers an email as spam in the local Bogofilter database.
func TrainSpam(emailBytes []byte) error {
	return trainBogofilter(emailBytes, "-s", "spam")
}

// trainBogofilter feeds emailBytes to bogofilter with flag (-n for ham, -s for
// spam), naming the training kind in any error so the two callers stay distinct.
func trainBogofilter(emailBytes []byte, flag, kind string) error {
	ctx, cancel := context.WithTimeout(context.Background(), bogofilterTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bogofilter", flag)
	cmd.Stdin = bytes.NewReader(emailBytes)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if ctx.Err() != nil {
		return fmt.Errorf("bogofilter %s training timed out after %s", kind, bogofilterTimeout)
	}
	if err != nil {
		return fmt.Errorf("bogofilter %s training failed: %w (stderr: %s)", kind, err, strings.TrimSpace(errBuf.String()))
	}
	return nil
}

// CheckSpamScore evaluates an email with Bogofilter and returns its spam score and verdict.
func CheckSpamScore(emailBytes []byte) (float32, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), bogofilterTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bogofilter", "-v")
	cmd.Stdin = bytes.NewReader(emailBytes)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	if ctx.Err() != nil {
		return 0, false, fmt.Errorf("bogofilter timed out after %s", bogofilterTimeout)
	}
	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			return 0, false, fmt.Errorf("running bogofilter: %w (stderr: %s)", runErr, strings.TrimSpace(errBuf.String()))
		}
	}

	outStr := outBuf.String()
	score := parseSpamicity(outStr)
	isSpam := strings.Contains(outStr, "Spam") || score >= 0.5
	return score, isSpam, nil
}
