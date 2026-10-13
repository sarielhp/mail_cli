package runner

import (
	"bytes"
	"fmt"
	"io"
	"net/mail"
	"os/exec"
	"strings"

	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/cli"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/report"
)

// RunPipe pipes a message's content (raw, body-only, or headers-only) to an external command.
func RunPipe(out io.Writer, errOut io.Writer, config *cfg_g.Config, opts cli.PipeOptions, cmdArgs []string, rep report.Reporter) error {
	if len(cmdArgs) == 0 {
		return fmt.Errorf("pipe requires an external command to execute")
	}

	_, _, _, rawBytes, err := findMessageAcrossAccounts(config, opts.Account, opts.MessageID, rep)
	if err != nil {
		return err
	}

	payload, err := extractPipePayload(rawBytes, opts)
	if err != nil {
		return fmt.Errorf("extracting message payload: %w", err)
	}

	cmd, err := buildPipeCmd(cmdArgs)
	if err != nil {
		return err
	}

	cmd.Stdin = bytes.NewReader(payload)
	cmd.Stdout = out
	cmd.Stderr = errOut

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command %q failed: %w", strings.Join(cmdArgs, " "), err)
	}
	return nil
}

func extractPipePayload(rawBytes []byte, opts cli.PipeOptions) ([]byte, error) {
	if opts.BodyOnly {
		msg, err := mail.ReadMessage(bytes.NewReader(rawBytes))
		if err != nil {
			return nil, fmt.Errorf("parsing message for body extraction: %w", err)
		}
		text, err := email.ExtractPlainBodyText(msg)
		if err != nil {
			return nil, fmt.Errorf("extracting plain body text: %w", err)
		}
		return []byte(text), nil
	}

	if opts.HeadersOnly {
		return extractHeadersOnly(rawBytes), nil
	}

	return rawBytes, nil
}

func extractHeadersOnly(rawBytes []byte) []byte {
	if idx := bytes.Index(rawBytes, []byte("\r\n\r\n")); idx != -1 {
		return rawBytes[:idx+4]
	}
	if idx := bytes.Index(rawBytes, []byte("\n\n")); idx != -1 {
		return rawBytes[:idx+2]
	}
	return rawBytes
}

func buildPipeCmd(cmdArgs []string) (*exec.Cmd, error) {
	if len(cmdArgs) == 0 {
		return nil, fmt.Errorf("empty command arguments")
	}
	if len(cmdArgs) == 1 && strings.ContainsAny(cmdArgs[0], " \t\n|&;<>()$`\\\"'") {
		return exec.Command("sh", "-c", cmdArgs[0]), nil
	}
	return exec.Command(cmdArgs[0], cmdArgs[1:]...), nil
}
