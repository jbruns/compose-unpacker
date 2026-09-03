package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadStoresVerifiedExecutableAtomically(t *testing.T) {
	t.Parallel()

	const downloaded = "downloaded bytes"
	server := newDownloadServer(t, http.StatusOK, downloaded)
	destination := filepath.Join(t.TempDir(), "sops")
	writeFile(t, destination, "old bytes", 0o700)

	err := Download(
		context.Background(),
		server.Client(),
		server.URL,
		destination,
		checksum(downloaded),
	)
	if err != nil {
		t.Fatal(err)
	}

	assertFile(t, destination, downloaded, 0o755)
	assertNoTemporaryFiles(t, filepath.Dir(destination))
}

func TestDownloadRejectsNonOKStatusWithoutReplacingDestination(t *testing.T) {
	t.Parallel()

	const responseBody = "sensitive upstream response"
	server := newDownloadServer(t, http.StatusBadGateway, responseBody)
	destination := filepath.Join(t.TempDir(), "sops")
	writeFile(t, destination, "old bytes", 0o700)

	err := Download(
		context.Background(),
		server.Client(),
		server.URL,
		destination,
		checksum(responseBody),
	)
	if err == nil || !strings.Contains(err.Error(), "status 502") {
		t.Fatalf("Download() error = %v, want status 502", err)
	}
	assertErrorContext(t, err, server.URL, destination)
	if strings.Contains(err.Error(), responseBody) {
		t.Fatalf("Download() error includes response body: %v", err)
	}
	assertFile(t, destination, "old bytes", 0o700)
	assertNoTemporaryFiles(t, filepath.Dir(destination))
}

func TestDownloadReportsNetworkFailureWithoutCreatingDestination(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	sourceURL := server.URL
	server.Close()
	destination := filepath.Join(t.TempDir(), "sops")

	err := Download(
		context.Background(),
		server.Client(),
		sourceURL,
		destination,
		checksum(""),
	)
	if err == nil {
		t.Fatal("Download() error = nil, want network failure")
	}
	assertErrorContext(t, err, sourceURL, destination)
	if _, statErr := os.Stat(destination); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("destination exists after network failure: %v", statErr)
	}
	assertNoTemporaryFiles(t, filepath.Dir(destination))
}

func TestDownloadRejectsChecksumMismatch(t *testing.T) {
	t.Parallel()

	server := newDownloadServer(t, http.StatusOK, "downloaded bytes")
	destination := filepath.Join(t.TempDir(), "sops")

	err := Download(
		context.Background(),
		server.Client(),
		server.URL,
		destination,
		strings.Repeat("0", 64),
	)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Download() error = %v, want checksum mismatch", err)
	}
	assertErrorContext(t, err, server.URL, destination)
	if _, statErr := os.Stat(destination); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("destination exists after mismatch: %v", statErr)
	}
	assertNoTemporaryFiles(t, filepath.Dir(destination))
}

func newDownloadServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}

func checksum(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func writeFile(t *testing.T, path, contents string, mode os.FileMode) {
	t.Helper()

	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, path, contents string, mode os.FileMode) {
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
	if gotMode := info.Mode().Perm(); gotMode != mode {
		t.Fatalf("%s mode = %o, want %o", path, gotMode, mode)
	}
}

func assertErrorContext(t *testing.T, err error, sourceURL, destination string) {
	t.Helper()

	if !strings.Contains(err.Error(), strings.TrimPrefix(sourceURL, "http://")) {
		t.Fatalf("Download() error = %v, want URL host", err)
	}
	if !strings.Contains(err.Error(), destination) {
		t.Fatalf("Download() error = %v, want destination", err)
	}
}

func assertNoTemporaryFiles(t *testing.T, directory string) {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(directory, ".sops.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files remain: %v", matches)
	}
}
