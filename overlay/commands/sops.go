package commands

import (
	"context"
	"fmt"

	"github.com/portainer/compose-unpacker/exec"
	"github.com/portainer/compose-unpacker/sopsdecrypt"
)

var decryptSOPS = sopsdecrypt.Decrypt

func decryptSOPSFiles(ctx context.Context, composeFiles, env []string) error {
	if err := decryptSOPS(ctx, composeFiles, env, sopsdecrypt.DefaultBinaryPath); err != nil {
		return fmt.Errorf("%w: decrypt SOPS files: %w", exec.ErrDeployComposeFailure, err)
	}
	return nil
}
