package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jbruns/compose-unpacker-sops/internal/manifest"
	"github.com/jbruns/compose-unpacker-sops/internal/update"
)

const (
	cliServerCommit   = "1111111111111111111111111111111111111111"
	cliUnpackerCommit = "2222222222222222222222222222222222222222"
	cliDigest         = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	cliSOPSSHA        = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestRunRejectsInvalidModeSelection(t *testing.T) {
	tests := [][]string{
		nil,
		{"-check", "-write"},
		{"-unknown"},
		{"-check", "extra"},
	}

	for _, args := range tests {
		args := args
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			code := run(args, &stdout, &stderr, cliSources{})
			if code != 1 {
				t.Fatalf("run() code = %d, want 1", code)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
			if stderr.Len() == 0 {
				t.Fatal("stderr is empty, want usage error")
			}
		})
	}
}

func TestRunCheckReturnsZeroWhenCurrent(t *testing.T) {
	path, before := writeManifestFixture(t, cliManifest())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"-manifest", path, "-check"}, &stdout, &stderr, currentCLISources())
	if code != 0 {
		t.Fatalf("run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	assertSummary(t, stdout.Bytes(), update.ChangeSummary{
		PortainerBefore: "2.45.0",
		PortainerAfter:  "2.45.0",
		SOPSBefore:      "v3.13.3",
		SOPSAfter:       "v3.13.3",
		OverlayRevision: 4,
	})
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("check mode changed manifest bytes:\n%s", after)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunCheckReturnsTwoWithoutWritingWhenUpdateExists(t *testing.T) {
	path, before := writeManifestFixture(t, cliManifest())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"-manifest", path, "-check"}, &stdout, &stderr, newerCLISources())
	if code != 2 {
		t.Fatalf("run() code = %d, want 2; stderr = %q", code, stderr.String())
	}
	assertSummary(t, stdout.Bytes(), update.ChangeSummary{
		Changed:         true,
		PortainerBefore: "2.45.0",
		PortainerAfter:  "2.46.0",
		SOPSBefore:      "v3.13.3",
		SOPSAfter:       "v3.14.0",
		OverlayRevision: 1,
	})
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("check mode changed manifest bytes:\n%s", after)
	}
}

func TestRunWriteAtomicallyFormatsUpdatedManifest(t *testing.T) {
	path, _ := writeManifestFixture(t, cliManifest())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"-manifest", path, "-write"}, &stdout, &stderr, newerCLISources())
	if code != 0 {
		t.Fatalf("run() code = %d, want 0; stderr = %q", code, stderr.String())
	}

	got, err := manifest.Load(path)
	if err != nil {
		t.Fatalf("Load(updated manifest) error = %v", err)
	}
	if got.Portainer.Version != "2.46.0" || got.SOPS.Version != "v3.14.0" || got.OverlayRevision != 1 {
		t.Fatalf("updated manifest = %#v", got)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	formatted, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	formatted = append(formatted, '\n')
	if !bytes.Equal(contents, formatted) {
		t.Fatalf("manifest is not deterministic formatted JSON:\n%s", contents)
	}
	assertNoTemporaryFiles(t, filepath.Dir(path))
}

func TestRunWritePreservesBytesWhenNothingChanges(t *testing.T) {
	path, before := writeManifestFixture(t, cliManifest())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"-manifest", path, "-write"}, &stdout, &stderr, currentCLISources())
	if code != 0 {
		t.Fatalf("run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("no-change write replaced manifest bytes:\n%s", after)
	}
	assertNoTemporaryFiles(t, filepath.Dir(path))
}

func TestRunCleansTemporaryFileAfterRenameFailure(t *testing.T) {
	path, before := writeManifestFixture(t, cliManifest())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	renameFailure := func(oldPath, newPath string) error {
		if filepath.Dir(oldPath) != filepath.Dir(newPath) {
			t.Errorf("temporary file directory = %q, destination directory = %q", filepath.Dir(oldPath), filepath.Dir(newPath))
		}
		return errors.New("forced rename failure")
	}
	code := runWithRename(
		[]string{"-manifest", path, "-write"},
		&stdout,
		&stderr,
		newerCLISources(),
		renameFailure,
	)
	if code != 1 {
		t.Fatalf("runWithRename() code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "rename manifest") {
		t.Fatalf("stderr = %q, want rename error", stderr.String())
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("failed write changed original manifest:\n%s", after)
	}
	assertNoTemporaryFiles(t, filepath.Dir(path))
}

func TestRunReturnsOneForManifestAndSourceErrors(t *testing.T) {
	t.Run("invalid manifest", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "versions.json")
		if err := os.WriteFile(path, []byte(`{"unknown":true}`), 0o600); err != nil {
			t.Fatal(err)
		}

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		if code := run([]string{"-manifest", path, "-check"}, &stdout, &stderr, currentCLISources()); code != 1 {
			t.Fatalf("run() code = %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), "decode") {
			t.Fatalf("stderr = %q, want manifest error", stderr.String())
		}
	})

	t.Run("source failure", func(t *testing.T) {
		path, _ := writeManifestFixture(t, cliManifest())
		sources := currentCLISources()
		sources.releaseErr = errors.New("API unavailable")

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		if code := run([]string{"-manifest", path, "-check"}, &stdout, &stderr, sources); code != 1 {
			t.Fatalf("run() code = %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), "resolve Portainer releases") {
			t.Fatalf("stderr = %q, want source error", stderr.String())
		}
	})
}

type cliSources struct {
	releases   []update.Release
	releaseErr error
	commits    map[string]string
	digest     string
	sops       update.SOPSRelease
}

func (s cliSources) PortainerReleases(context.Context) ([]update.Release, error) {
	return s.releases, s.releaseErr
}

func (s cliSources) TagCommit(_ context.Context, repository, tag string) (string, error) {
	commit, ok := s.commits[repository+"/"+tag]
	if !ok {
		return "", errors.New("tag not found")
	}
	return commit, nil
}

func (s cliSources) LinuxAMD64Digest(context.Context, string, string) (string, error) {
	return s.digest, nil
}

func (s cliSources) LatestSOPS(context.Context) (update.SOPSRelease, error) {
	return s.sops, nil
}

func currentCLISources() cliSources {
	return cliSources{
		releases: []update.Release{
			{TagName: "2.45.0", Name: "Release 2.45.0 LTS"},
		},
		commits: map[string]string{
			"portainer/2.45.0":        cliServerCommit,
			"compose-unpacker/2.45.0": cliUnpackerCommit,
		},
		digest: cliDigest,
		sops: update.SOPSRelease{
			Version: "v3.13.3",
			Asset:   "sops-v3.13.3.linux.amd64",
			URL:     "https://github.com/getsops/sops/releases/download/v3.13.3/sops-v3.13.3.linux.amd64",
			SHA256:  cliSOPSSHA,
		},
	}
}

func newerCLISources() cliSources {
	return cliSources{
		releases: []update.Release{
			{TagName: "2.45.0", Name: "Release 2.45.0 LTS"},
			{TagName: "2.46.0", Name: "Release 2.46.0 LTS"},
		},
		commits: map[string]string{
			"portainer/2.46.0":        "3333333333333333333333333333333333333333",
			"compose-unpacker/2.46.0": "4444444444444444444444444444444444444444",
		},
		digest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		sops: update.SOPSRelease{
			Version: "v3.14.0",
			Asset:   "sops-v3.14.0.linux.amd64",
			URL:     "https://github.com/getsops/sops/releases/download/v3.14.0/sops-v3.14.0.linux.amd64",
			SHA256:  "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		},
	}
}

func cliManifest() manifest.Manifest {
	return manifest.Manifest{
		Portainer: manifest.Portainer{
			Version:               "2.45.0",
			ComposeUnpackerCommit: cliUnpackerCommit,
			ServerCommit:          cliServerCommit,
			Image:                 "docker.io/portainer/compose-unpacker",
			LinuxAMD64Digest:      cliDigest,
		},
		Build: manifest.Build{
			GoVersion:           "1.26.6",
			GolangCILintVersion: "v2.13.2",
		},
		SOPS: manifest.SOPS{
			Version: "v3.13.3",
			Asset:   "sops-v3.13.3.linux.amd64",
			URL:     "https://github.com/getsops/sops/releases/download/v3.13.3/sops-v3.13.3.linux.amd64",
			SHA256:  cliSOPSSHA,
		},
		Platform:        "linux/amd64",
		OverlayRevision: 4,
	}
}

func writeManifestFixture(t *testing.T, value manifest.Manifest) (string, []byte) {
	t.Helper()

	contents, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "versions.json")
	if err := os.WriteFile(path, contents, 0o640); err != nil {
		t.Fatal(err)
	}
	return path, contents
}

func assertSummary(t *testing.T, contents []byte, want update.ChangeSummary) {
	t.Helper()

	var got update.ChangeSummary
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode summary: %v; output = %q", err, contents)
	}
	if got != want {
		t.Fatalf("summary = %#v, want %#v", got, want)
	}
}

func assertNoTemporaryFiles(t *testing.T, directory string) {
	t.Helper()

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "versions.json" {
			t.Fatalf("unexpected file left after write: %s", entry.Name())
		}
	}
}
