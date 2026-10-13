package pgp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	maxGPGOutputBytes = 64 << 20 // 64 MB
	maxGPGStderrBytes = 4 << 20  // 4 MB
	maxGPGStatusBytes = 4 << 20  // 4 MB
	gpgTimeout        = 60 * time.Second
	// statusDrainGrace bounds the wait for the status pipe after gpg exits: a
	// helper that inherited the descriptor must not hold a read open forever.
	statusDrainGrace = 5 * time.Second
)

// errOutputLimit reports output that went past its cap. What was kept is a
// prefix, and a prefix of a decryption is not the message.
var errOutputLimit = errors.New("gpg output limit exceeded")

// limitedWriter keeps the first max bytes and discards the rest.
//
// It keeps accepting writes after the cap: returning an error stops the copy
// that drains gpg's pipe, and gpg then blocks on a full pipe until the
// deadline kills it. exceeded records that something was dropped.
type limitedWriter struct {
	buf      *bytes.Buffer
	max      int
	exceeded bool
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	remaining := w.max - w.buf.Len()
	if len(p) <= remaining {
		return w.buf.Write(p)
	}
	w.exceeded = true
	if remaining > 0 {
		w.buf.Write(p[:remaining])
	}
	return len(p), nil
}

// ErrUnavailable reports that no gpg binary is on PATH. Callers treat it as
// "this message cannot be read here", not as a message that failed.
var ErrUnavailable = errors.New("gpg is not installed")

// baseArgs are on every invocation.
//
// --batch keeps gpg from asking questions of its own; a passphrase still goes
// through pinentry, which is a separate program and is what makes a protected
// key usable without this program ever seeing the passphrase.
//
// The status channel is descriptor 3, a pipe of its own. On stderr it shared a
// stream with gpg's prose, which repeats text the message controls -- a file
// name, a user id -- and that is how a crafted message forges a status line
// (CVE-2018-12020). Nothing read from any other descriptor is parsed as status.
const baseArgs = "--batch --status-fd 3"

// gpgResult is one invocation's outcome.
//
// ExitErr is kept apart from the status because a non-zero exit is how gpg
// reports BADSIG: for a question, the exit code is an answer and not a failure.
// LimitErr is set when any output went past its cap and overrides everything
// else: a truncated answer is not an answer.
type gpgResult struct {
	Stdout   []byte
	Status   Status
	Diag     string
	ExitErr  error
	LimitErr error
}

// execGPG runs gpg with stdin, returning its stdout and parsed status. It
// returns an error only when gpg could not be found; a non-zero exit is
// reported through ExitErr so the caller can decide what it means.
func execGPG(ctx context.Context, stdin []byte, args ...string) (gpgResult, error) {
	path, err := exec.LookPath("gpg")
	if err != nil {
		return gpgResult{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, gpgTimeout)
	defer cancel()

	statusR, statusW, err := os.Pipe()
	if err != nil {
		return gpgResult{ExitErr: err}, nil
	}
	defer func() { _ = statusR.Close() }()

	full := append(strings.Fields(baseArgs), args...)
	cmd := exec.CommandContext(ctx, path, full...)
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.ExtraFiles = []*os.File{statusW} // descriptor 3 in the child
	var stdout, stderr, status bytes.Buffer
	outW := &limitedWriter{buf: &stdout, max: maxGPGOutputBytes}
	errW := &limitedWriter{buf: &stderr, max: maxGPGStderrBytes}
	statW := &limitedWriter{buf: &status, max: maxGPGStatusBytes}
	cmd.Stdout, cmd.Stderr = outW, errW

	startErr := cmd.Start()
	_ = statusW.Close() // the child holds its own copy
	if startErr != nil {
		return gpgResult{ExitErr: startErr}, nil
	}
	drained := make(chan struct{})
	go func() {
		_, _ = io.Copy(statW, statusR)
		close(drained)
	}()
	runErr := cmd.Wait()
	awaitDrain(drained, statusR)

	res := gpgResult{
		Stdout:  stdout.Bytes(),
		Status:  parseStatus(status.String()),
		Diag:    diagnostics(stderr.String()),
		ExitErr: runErr,
	}
	if outW.exceeded || errW.exceeded || statW.exceeded {
		res.LimitErr = errOutputLimit
	}
	return res, nil
}

// awaitDrain waits for the status copy to finish once gpg has exited, closing
// the pipe when something else still holds its write end.
func awaitDrain(drained <-chan struct{}, r *os.File) {
	select {
	case <-drained:
	case <-time.After(statusDrainGrace):
		_ = r.Close()
		<-drained
	}
}

// diagnostics is gpg's own prose, joined into one line for an error message.
// It is never parsed: the status channel is a different descriptor.
func diagnostics(stderr string) string {
	var lines []string
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "; ")
}

// decisive reports whether the status channel already answered the question, so
// a non-zero exit is the answer rather than a failure.
func (s Status) decisive() bool {
	return s.Signed() || s.NoData || s.Decrypted || s.DecryptFailed
}

// failure turns a gpg result into an error for operations where a non-zero exit
// is not an answer.
func (r gpgResult) failure(op string) error {
	if r.LimitErr != nil {
		return errors.New(op + ": " + r.LimitErr.Error())
	}
	if r.ExitErr == nil {
		return nil
	}
	message := r.Diag
	if message == "" {
		message = r.ExitErr.Error()
	}
	return errors.New(op + ": " + message)
}

// Available reports whether a usable gpg binary is on PATH.
func Available() bool {
	_, err := exec.LookPath("gpg")
	return err == nil
}
