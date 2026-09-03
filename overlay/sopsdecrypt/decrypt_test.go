//nolint:forbidigo // Test paths are rooted in t.TempDir.
package sopsdecrypt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecryptFilesAtomicallyReplacesDestination(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	source := filepath.Join(root, "app.sops.env")
	destination := filepath.Join(root, "app.env")
	writeFile(t, source, []byte("encrypted"), 0o600)
	writeFile(t, destination, []byte("old"), 0o644)

	err := decryptFiles(
		context.Background(),
		[]encryptedFile{{Source: source, Destination: destination}},
		[]string{"SOPS_AGE_KEY=secret-test-key"},
		fakeRunner{plaintext: []byte("VALUE=decrypted\n")},
		defaultFileOps(),
	)
	if err != nil {
		t.Fatal(err)
	}
	assertFile(t, destination, "VALUE=decrypted\n", 0o600)
	assertNoTemporaryFiles(t, root)
}

func TestDecryptFilesReplacesSymlinkWithoutModifyingTarget(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	source := filepath.Join(root, "app.sops.env")
	destination := filepath.Join(root, "app.env")
	target := filepath.Join(root, "target.env")
	writeFile(t, source, []byte("encrypted"), 0o600)
	writeFile(t, target, []byte("target"), 0o644)
	if err := os.Symlink(target, destination); err != nil {
		t.Fatal(err)
	}

	err := decryptFiles(
		context.Background(),
		[]encryptedFile{{Source: source, Destination: destination}},
		nil,
		fakeRunner{plaintext: []byte("decrypted")},
		defaultFileOps(),
	)
	if err != nil {
		t.Fatal(err)
	}
	assertFile(t, destination, "decrypted", 0o600)
	assertFile(t, target, "target", 0o644)
	if info, err := os.Lstat(destination); err != nil {
		t.Fatal(err)
	} else if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("%s remains a symlink", destination)
	}
}

func TestDecryptFilesRemovesTemporaryFilesOnFailures(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("injected failure")
	tests := []struct {
		name      string
		configure func(*fileOps, *runner)
	}{
		{
			name: "process",
			configure: func(_ *fileOps, current *runner) {
				*current = fakeRunner{err: sentinel}
			},
		},
		{
			name: "write",
			configure: func(_ *fileOps, current *runner) {
				*current = runnerFunc(func(_ context.Context, _ string, destination io.Writer, _ []string) error {
					file := destination.(*os.File)
					if err := file.Close(); err != nil {
						return err
					}
					_, err := destination.Write([]byte("plaintext"))
					return fmt.Errorf("write plaintext: %w", err)
				})
			},
		},
		{
			name: "sync",
			configure: func(ops *fileOps, _ *runner) {
				ops.Sync = func(*os.File) error { return sentinel }
			},
		},
		{
			name: "close",
			configure: func(ops *fileOps, _ *runner) {
				ops.Close = func(file *os.File) error {
					if err := file.Close(); err != nil {
						return err
					}
					return sentinel
				}
			},
		},
		{
			name: "rename",
			configure: func(ops *fileOps, _ *runner) {
				ops.Rename = func(string, string) error { return sentinel }
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			source := filepath.Join(root, "app.sops.env")
			destination := filepath.Join(root, "app.env")
			writeFile(t, source, []byte("encrypted"), 0o600)

			ops := defaultFileOps()
			var current runner = fakeRunner{plaintext: []byte("decrypted")}
			test.configure(&ops, &current)

			err := decryptFiles(
				context.Background(),
				[]encryptedFile{{Source: source, Destination: destination}},
				nil,
				current,
				ops,
			)
			if err == nil {
				t.Fatal("decryptFiles() error = nil")
			}
			if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("Lstat(%q) error = %v, want not exist", destination, err)
			}
			assertNoTemporaryFiles(t, root)
		})
	}
}

func TestDecryptFilesCreatesNoDestinationAfterRunnerFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	source := filepath.Join(root, "app.sops.env")
	destination := filepath.Join(root, "app.env")
	writeFile(t, source, []byte("encrypted"), 0o600)
	sentinel := errors.New("runner failed")

	err := decryptFiles(
		context.Background(),
		[]encryptedFile{{Source: source, Destination: destination}},
		nil,
		fakeRunner{err: sentinel},
		defaultFileOps(),
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("decryptFiles() error = %v, want %v", err, sentinel)
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Lstat(%q) error = %v, want not exist", destination, err)
	}
	assertNoTemporaryFiles(t, root)
}

func TestDecryptFilesStopsWhenContextIsCanceled(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	source := filepath.Join(root, "app.sops.env")
	destination := filepath.Join(root, "app.env")
	writeFile(t, source, []byte("encrypted"), 0o600)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	current := runnerFunc(func(context.Context, string, io.Writer, []string) error {
		called = true
		return nil
	})

	err := decryptFiles(
		ctx,
		[]encryptedFile{{Source: source, Destination: destination}},
		nil,
		current,
		defaultFileOps(),
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("decryptFiles() error = %v, want context canceled", err)
	}
	if called {
		t.Fatal("runner called after context cancellation")
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Lstat(%q) error = %v, want not exist", destination, err)
	}
}

func TestDecryptFilesDoesNotPromoteAfterRunnerCancelsContext(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	source := filepath.Join(root, "app.sops.env")
	destination := filepath.Join(root, "app.env")
	writeFile(t, source, []byte("encrypted"), 0o600)

	ctx, cancel := context.WithCancel(context.Background())
	current := runnerFunc(func(_ context.Context, _ string, output io.Writer, _ []string) error {
		if _, err := output.Write([]byte("plaintext")); err != nil {
			return err
		}
		cancel()
		return nil
	})

	err := decryptFiles(
		ctx,
		[]encryptedFile{{Source: source, Destination: destination}},
		nil,
		current,
		defaultFileOps(),
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("decryptFiles() error = %v, want context canceled", err)
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Lstat(%q) error = %v, want not exist", destination, err)
	}
	assertNoTemporaryFiles(t, root)
}

func TestDecryptDiscoversEncryptedFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	composeFile := filepath.Join(root, "compose.yaml")
	source := filepath.Join(root, "nested", "app.sops.env")
	destination := filepath.Join(root, "nested", "app.env")
	writeFile(t, composeFile, []byte("services: {}\n"), 0o600)
	writeFile(t, source, []byte("encrypted"), 0o600)
	env := []string{
		"SOPS_TEST_HELPER=1",
		"SOPS_TEST_ACTION=plaintext",
	}

	if err := Decrypt(context.Background(), []string{composeFile}, env, os.Args[0]); err != nil {
		t.Fatal(err)
	}
	assertFile(t, destination, "VALUE=decrypted\n", 0o600)
	assertNoTemporaryFiles(t, filepath.Dir(destination))
}

type fakeRunner struct {
	plaintext []byte
	err       error
}

func (r fakeRunner) Decrypt(_ context.Context, _ string, destination io.Writer, _ []string) error {
	if r.err != nil {
		return r.err
	}
	_, err := destination.Write(r.plaintext)
	return err
}

type runnerFunc func(context.Context, string, io.Writer, []string) error

func (fn runnerFunc) Decrypt(ctx context.Context, source string, destination io.Writer, env []string) error {
	return fn(ctx, source, destination, env)
}

func writeFile(t *testing.T, path string, contents []byte, mode fs.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, mode); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, path, contents string, mode fs.FileMode) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != contents {
		t.Fatalf("%s contents = %q, want %q", path, got, contents)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != mode {
		t.Fatalf("%s mode = %o, want %o", path, info.Mode().Perm(), mode)
	}
}

func assertNoTemporaryFiles(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Fatalf("temporary file %q was not removed", filepath.Join(directory, entry.Name()))
		}
	}
}
