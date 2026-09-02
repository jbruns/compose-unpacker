package sopsdecrypt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
)

const DefaultBinaryPath = "/app/sops"

type runner interface {
	Decrypt(ctx context.Context, source string, destination io.Writer, env []string) error
}

type binaryRunner struct {
	binary string
}

func (r binaryRunner) Decrypt(ctx context.Context, source string, destination io.Writer, env []string) error {
	absoluteSource, err := filepath.Abs(source)
	if err != nil {
		return fmt.Errorf("decrypt %s: resolve source: %w", source, err)
	}

	command := exec.CommandContext(ctx, r.binary, "decrypt", absoluteSource)
	command.Env = append([]string(nil), env...)
	command.Stdout = destination
	var stderr bytes.Buffer
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("decrypt %s: %w", absoluteSource, ctxErr)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("decrypt %s: sops exited with status %d", absoluteSource, exitErr.ExitCode())
		}
		return fmt.Errorf("decrypt %s: start sops: %w", absoluteSource, err)
	}

	return nil
}
