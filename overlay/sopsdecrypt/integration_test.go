//go:build integration

package sopsdecrypt

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecryptWithRealSOPSAge(t *testing.T) {
	sopsBinary := os.Getenv("SOPS_BINARY")
	if sopsBinary == "" {
		t.Fatal("SOPS_BINARY must name the pinned SOPS executable")
	}
	identity, err := os.ReadFile(filepath.Join("testdata", "age-key.txt"))
	if err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile(filepath.Join("testdata", "config.env.expected"))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("decrypts age fixture", func(t *testing.T) {
		composeDirectory, composeFile := newComposeDirectory(t)
		copyFixture(t, filepath.Join("testdata", "config.sops.env"), filepath.Join(composeDirectory, "config.sops.env"))

		err := Decrypt(
			context.Background(),
			[]string{composeFile},
			[]string{"SOPS_AGE_KEY=" + string(identity)},
			sopsBinary,
		)
		if err != nil {
			t.Fatal(err)
		}

		actual, err := os.ReadFile(filepath.Join(composeDirectory, "config.env"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(actual, expected) {
			t.Fatalf("plaintext = %q, want %q", actual, expected)
		}
		assertNoTemporaryFiles(t, composeDirectory)
	})

	t.Run("rejects invalid key", func(t *testing.T) {
		composeDirectory, composeFile := newComposeDirectory(t)
		source := filepath.Join(composeDirectory, "config.sops.env")
		copyFixture(t, filepath.Join("testdata", "config.sops.env"), source)

		err := Decrypt(
			context.Background(),
			[]string{composeFile},
			[]string{"SOPS_AGE_KEY=AGE-SECRET-KEY-1INVALID"},
			sopsBinary,
		)
		if err == nil || !strings.Contains(err.Error(), "run sops") {
			t.Fatalf("Decrypt() error = %v, want SOPS failure", err)
		}
		if _, statErr := os.Stat(filepath.Join(composeDirectory, "config.env")); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("plaintext exists after invalid key: %v", statErr)
		}
		assertNoTemporaryFiles(t, composeDirectory)
	})

	t.Run("rejects malformed input", func(t *testing.T) {
		composeDirectory, composeFile := newComposeDirectory(t)
		writeFile(
			t,
			filepath.Join(composeDirectory, "malformed.sops.env"),
			[]byte("NOT_SOPS=non-sensitive-test-content\n"),
			0o600,
		)

		err := Decrypt(
			context.Background(),
			[]string{composeFile},
			[]string{"SOPS_AGE_KEY=" + string(identity)},
			sopsBinary,
		)
		if err == nil || !strings.Contains(err.Error(), "run sops") {
			t.Fatalf("Decrypt() error = %v, want SOPS failure", err)
		}
		if _, statErr := os.Stat(filepath.Join(composeDirectory, "malformed.env")); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("plaintext exists after malformed input: %v", statErr)
		}
		assertNoTemporaryFiles(t, composeDirectory)
	})
}

func newComposeDirectory(t *testing.T) (string, string) {
	t.Helper()

	directory := t.TempDir()
	composeFile := filepath.Join(directory, "compose.yaml")
	writeFile(t, composeFile, []byte("services: {}\n"), 0o600)
	return directory, composeFile
}

func copyFixture(t *testing.T, source, destination string) {
	t.Helper()

	contents, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, destination, contents, 0o600)
}
