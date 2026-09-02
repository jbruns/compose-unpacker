package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPrintsManifestValues(t *testing.T) {
	t.Parallel()

	manifestPath := filepath.Join("..", "..", "versions.json")
	tests := map[string]string{
		"go-version":        "1.26.6",
		"lint-version":      "v2.13.2",
		"base-image":        "docker.io/portainer/compose-unpacker@sha256:25aea494af4f4f04ce46f9cf4c72e49ed21085cc80e63561cc75292da54bd60a",
		"base-digest":       "sha256:25aea494af4f4f04ce46f9cf4c72e49ed21085cc80e63561cc75292da54bd60a",
		"portainer-version": "2.45.0",
		"sops-version":      "v3.13.3",
		"overlay-revision":  "1",
		"immutable-tag":     "2.45.0-sops.1",
		"version-tag":       "2.45.0-sops",
	}

	for field, want := range tests {
		field, want := field, want
		t.Run(field, func(t *testing.T) {
			t.Parallel()

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			if code := run([]string{"-manifest", manifestPath, field}, &stdout, &stderr); code != 0 {
				t.Fatalf("run() code = %d, stderr = %q", code, stderr.String())
			}
			if got := stdout.String(); got != want+"\n" {
				t.Fatalf("stdout = %q, want %q", got, want+"\n")
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

func TestRunUsesDefaultManifestPath(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if code := run([]string{"portainer-version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() code = %d, stderr = %q", code, stderr.String())
	}
	if got := stdout.String(); got != "2.45.0\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestRunRejectsUnknownField(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if code := run([]string{"-manifest", filepath.Join("..", "..", "versions.json"), "unknown"}, &stdout, &stderr); code == 0 {
		t.Fatal("run() code = 0, want non-zero")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "unknown field") {
		t.Fatalf("stderr = %q, want unknown field error", stderr.String())
	}
}

func TestRunRejectsExtraArguments(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if code := run([]string{"-manifest", filepath.Join("..", "..", "versions.json"), "portainer-version", "extra"}, &stdout, &stderr); code == 0 {
		t.Fatal("run() code = 0, want non-zero")
	}
	if !strings.Contains(stderr.String(), "exactly one field") {
		t.Fatalf("stderr = %q, want argument error", stderr.String())
	}
}

func TestRunReportsLoadErrors(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	path := filepath.Join("..", "..", "internal", "manifest", "testdata", "unknown-field.json")
	if code := run([]string{"-manifest", path, "go-version"}, &stdout, &stderr); code == 0 {
		t.Fatal("run() code = 0, want non-zero")
	}
	if !strings.Contains(stderr.String(), "decode") {
		t.Fatalf("stderr = %q, want decode error", stderr.String())
	}
}
