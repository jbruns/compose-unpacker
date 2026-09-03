package sopsdecrypt

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type fileOps struct {
	CreateTemp func(directory, pattern string) (*os.File, error)
	Chmod      func(file *os.File, mode fs.FileMode) error
	Sync       func(file *os.File) error
	Close      func(file *os.File) error
	Rename     func(oldPath, newPath string) error
	Remove     func(path string) error
}

func Decrypt(ctx context.Context, composeFiles, env []string, binary string) error {
	files, err := discover(composeFiles, filepath.WalkDir)
	if err != nil {
		return fmt.Errorf("discover encrypted files: %w", err)
	}
	return decryptFiles(ctx, files, env, binaryRunner{binary: binary}, defaultFileOps())
}

func decryptFiles(ctx context.Context, files []encryptedFile, env []string, currentRunner runner, ops fileOps) error {
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("decrypt %s: check context: %w", file.Source, err)
		}
		if err := decryptFile(ctx, file, env, currentRunner, ops); err != nil {
			return err
		}
	}
	return nil
}

func decryptFile(ctx context.Context, file encryptedFile, env []string, currentRunner runner, ops fileOps) error {
	temporary, err := ops.CreateTemp(
		filepath.Dir(file.Destination),
		"."+filepath.Base(file.Destination)+".tmp-*",
	)
	if err != nil {
		return fmt.Errorf("decrypt %s: create temporary output: %w", file.Source, err)
	}

	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = temporary.Close()
			_ = ops.Remove(temporary.Name())
		}
	}()

	if err := ops.Chmod(temporary, 0o600); err != nil {
		return fmt.Errorf("decrypt %s: set temporary output mode: %w", file.Source, err)
	}
	if err := currentRunner.Decrypt(ctx, file.Source, temporary, append([]string(nil), env...)); err != nil {
		return fmt.Errorf("decrypt %s: run sops: %w", file.Source, err)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("decrypt %s: check context after sops: %w", file.Source, err)
	}
	if err := ops.Sync(temporary); err != nil {
		return fmt.Errorf("decrypt %s: sync temporary output: %w", file.Source, err)
	}
	if err := ops.Close(temporary); err != nil {
		return fmt.Errorf("decrypt %s: close temporary output: %w", file.Source, err)
	}
	if err := ops.Rename(temporary.Name(), file.Destination); err != nil {
		return fmt.Errorf("decrypt %s: promote temporary output: %w", file.Source, err)
	}

	removeTemporary = false
	return nil
}

func defaultFileOps() fileOps {
	return fileOps{
		CreateTemp: os.CreateTemp,
		Chmod: func(file *os.File, mode fs.FileMode) error {
			return file.Chmod(mode)
		},
		Sync: func(file *os.File) error {
			return file.Sync()
		},
		Close: func(file *os.File) error {
			return file.Close()
		},
		Rename: os.Rename,
		Remove: os.Remove,
	}
}
