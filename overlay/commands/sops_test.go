package commands

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/portainer/compose-unpacker/exec"
	"github.com/portainer/compose-unpacker/sopsdecrypt"
)

type contextKey string

func TestDecryptSOPSFilesPassesInputsAndWrapsFailure(t *testing.T) {
	wantCtx := context.WithValue(context.Background(), contextKey("request"), "task-4")
	wantComposeFiles := []string{
		"/stacks/demo/compose.yaml",
		"/stacks/demo/compose.override.yaml",
	}
	wantEnv := []string{"FOO=bar", "BAZ=qux"}

	decryptErr := errors.New("decrypt failed")
	var gotCtx context.Context
	var gotComposeFiles []string
	var gotEnv []string
	var gotBinary string

	decryptSOPS = func(ctx context.Context, composeFiles, env []string, binary string) error {
		gotCtx = ctx
		gotComposeFiles = append([]string(nil), composeFiles...)
		gotEnv = append([]string(nil), env...)
		gotBinary = binary
		return decryptErr
	}
	t.Cleanup(func() {
		decryptSOPS = sopsdecrypt.Decrypt
	})

	err := decryptSOPSFiles(wantCtx, wantComposeFiles, wantEnv)
	if gotCtx != wantCtx {
		t.Fatalf("decryptSOPSFiles() context = %v, want %v", gotCtx, wantCtx)
	}
	if !slices.Equal(gotComposeFiles, wantComposeFiles) {
		t.Fatalf("decryptSOPSFiles() compose files = %v, want %v", gotComposeFiles, wantComposeFiles)
	}
	if !slices.Equal(gotEnv, wantEnv) {
		t.Fatalf("decryptSOPSFiles() env = %v, want %v", gotEnv, wantEnv)
	}
	if gotBinary != sopsdecrypt.DefaultBinaryPath {
		t.Fatalf("decryptSOPSFiles() binary = %q, want %q", gotBinary, sopsdecrypt.DefaultBinaryPath)
	}
	if !errors.Is(err, exec.ErrDeployComposeFailure) {
		t.Fatalf("decryptSOPSFiles() error = %v, want wrapped %v", err, exec.ErrDeployComposeFailure)
	}
}
