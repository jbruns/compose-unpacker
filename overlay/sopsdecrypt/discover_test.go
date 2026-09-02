//nolint:forbidigo // Test paths are rooted in t.TempDir or fixed fixtures.
package sopsdecrypt

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRootDirectoriesReduceNestedAndDuplicateInputs(t *testing.T) {
	t.Parallel()

	got, err := rootDirectories([]string{
		"/stacks/b/nested/compose.yaml",
		"/stacks/a/compose.yaml",
		"/stacks/b/compose.yaml",
		"/stacks/a/compose.yaml",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/stacks/a", "/stacks/b"}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("roots = %v, want %v", got, want)
	}
}

func TestRootDirectoriesPreservePathBoundaries(t *testing.T) {
	t.Parallel()

	got, err := rootDirectories([]string{
		"/stacks/a/compose.yaml",
		"/stacks/a/nested/compose.yaml",
		"/stacks/ab/compose.yaml",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/stacks/a", "/stacks/ab"}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("roots = %v, want %v", got, want)
	}
}

func TestDiscoverRecursesSortsAndExcludesGitAndSymlinks(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeDiscoveryFile(t, filepath.Join(root, "z.sops.env"))
	writeDiscoveryFile(t, filepath.Join(root, "nested", "a.sops.yaml"))
	writeDiscoveryFile(t, filepath.Join(root, "nested", "ordinary.env"))
	writeDiscoveryFile(t, filepath.Join(root, ".git", "ignored.sops.env"))

	outside := t.TempDir()
	writeDiscoveryFile(t, filepath.Join(outside, "target.sops.env"))
	if err := os.Symlink(filepath.Join(outside, "target.sops.env"), filepath.Join(root, "linked.sops.env")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked-directory")); err != nil {
		t.Fatal(err)
	}

	got, err := discover([]string{filepath.Join(root, "compose.yaml")}, filepath.WalkDir)
	if err != nil {
		t.Fatal(err)
	}
	want := []encryptedFile{
		{
			Source:      filepath.Join(root, "nested", "a.sops.yaml"),
			Destination: filepath.Join(root, "nested", "a.yaml"),
		},
		{
			Source:      filepath.Join(root, "z.sops.env"),
			Destination: filepath.Join(root, "z.env"),
		},
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("files = %#v, want %#v", got, want)
	}
}

func TestDiscoverRejectsDuplicateDestinations(t *testing.T) {
	t.Parallel()

	first := "/repo/a.sops.b.sops.env"
	second := "/repo/a.b.sops.sops.env"
	walk := func(root string, fn fs.WalkDirFunc) error {
		for _, path := range []string{first, second} {
			if err := fn(path, regularDirEntry{name: filepath.Base(path)}, nil); err != nil {
				return err
			}
		}
		return nil
	}

	_, err := discover([]string{"/repo/compose.yaml"}, walk)
	if err == nil {
		t.Fatal("discover() error = nil, want duplicate destination error")
	}
	if !strings.Contains(err.Error(), first) || !strings.Contains(err.Error(), second) {
		t.Fatalf("error %q does not name both colliding sources", err)
	}
}

func TestDiscoverPropagatesWalkErrors(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("walk failed")
	walk := func(string, fs.WalkDirFunc) error {
		return sentinel
	}

	_, err := discover([]string{"/repo/compose.yaml"}, walk)
	if !errors.Is(err, sentinel) {
		t.Fatalf("discover() error = %v, want %v", err, sentinel)
	}
}

func TestDiscoverPropagatesCallbackErrors(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("entry failed")
	walk := func(root string, fn fs.WalkDirFunc) error {
		return fn(filepath.Join(root, "secret.sops.env"), nil, sentinel)
	}

	_, err := discover([]string{"/repo/compose.yaml"}, walk)
	if !errors.Is(err, sentinel) {
		t.Fatalf("discover() error = %v, want %v", err, sentinel)
	}
}

func TestOutputPathReplacesFirstMarker(t *testing.T) {
	t.Parallel()

	got, err := outputPath("/repo/app.sops.env")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/repo/app.env" {
		t.Fatalf("outputPath() = %q", got)
	}
}

func TestOutputPathRequiresMarkerInBaseName(t *testing.T) {
	t.Parallel()

	for _, source := range []string{
		"/repo/sops.env",
		"/repo/app.sops",
		"/repo/.sops-directory/app.env",
	} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			if _, err := outputPath(source); err == nil {
				t.Fatalf("outputPath(%q) error = nil", source)
			}
		})
	}
}

func writeDiscoveryFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
}

type regularDirEntry struct {
	name string
}

func (e regularDirEntry) Name() string               { return e.name }
func (regularDirEntry) IsDir() bool                  { return false }
func (regularDirEntry) Type() fs.FileMode            { return 0 }
func (e regularDirEntry) Info() (fs.FileInfo, error) { return regularFileInfo(e), nil }

type regularFileInfo struct {
	name string
}

func (i regularFileInfo) Name() string     { return i.name }
func (regularFileInfo) Size() int64        { return 0 }
func (regularFileInfo) Mode() fs.FileMode  { return 0o600 }
func (regularFileInfo) ModTime() time.Time { return time.Time{} }
func (regularFileInfo) IsDir() bool        { return false }
func (regularFileInfo) Sys() any           { return nil }
