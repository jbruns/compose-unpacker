package prepare

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jbruns/compose-unpacker-sops/internal/manifest"
)

func TestRunPreparesImmutableUpstreamTrees(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workDir := filepath.Join(root, ".work")
	keepFile := filepath.Join(root, "keep", "sentinel.txt")
	if err := os.MkdirAll(filepath.Dir(keepFile), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workDir, "stale"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(keepFile, []byte("keep"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "stale", "old.txt"), []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "patches"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	patchPath := filepath.Join(root, "patches", "compose-unpacker.patch")
	if err := os.WriteFile(patchPath, []byte("placeholder"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	currentManifest := validManifest()
	composeDir := filepath.Join(workDir, "upstream", "compose-unpacker")
	portainerDir := filepath.Join(workDir, "upstream", "portainer")

	runner := newFakeRunner()
	runner.setOutput(composeDir, "git rev-parse HEAD", currentManifest.Portainer.ComposeUnpackerCommit+"\n")
	runner.setOutput(portainerDir, "git rev-parse HEAD", currentManifest.Portainer.ServerCommit+"\n")
	runner.setOutput(composeDir, "git status --porcelain", " M commands/compose_deploy.go\n M go.mod\n")
	runner.setOutput(portainerDir, "git status --porcelain", "")

	if err := Run(context.Background(), Options{
		Root:     root,
		WorkDir:  workDir,
		Manifest: currentManifest,
	}, runner); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if _, err := os.Stat(keepFile); err != nil {
		t.Fatalf("Stat(%q) error = %v, want file kept", keepFile, err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "stale")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat(%q) error = %v, want not exist", filepath.Join(workDir, "stale"), err)
	}

	wantCommands := []string{
		"git init",
		"git remote add origin https://github.com/portainer/compose-unpacker.git",
		"git fetch --depth=1 origin refs/tags/2.45.0",
		"git checkout --detach 23c8e42176c521cb6745b3ea95233d3a68bbe031",
		"git init",
		"git remote add origin https://github.com/portainer/portainer.git",
		"git fetch --depth=1 origin refs/tags/2.45.0",
		"git checkout --detach d79ba726cd54395a54cca5e9180609ce52fa7a4f",
		"go mod edit -replace=github.com/portainer/portainer=../portainer",
		"git apply --check " + patchPath,
		"git apply " + patchPath,
	}
	if !reflect.DeepEqual(runner.runCommands(), wantCommands) {
		t.Fatalf("run commands = %#v, want %#v", runner.runCommands(), wantCommands)
	}

	wantOutputCommands := []string{
		"git rev-parse HEAD",
		"git rev-parse HEAD",
		"git status --porcelain",
		"git status --porcelain",
	}
	if !reflect.DeepEqual(runner.outputCommands(), wantOutputCommands) {
		t.Fatalf("output commands = %#v, want %#v", runner.outputCommands(), wantOutputCommands)
	}
}

func TestRunRejectsOverlayOverwrite(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workDir := filepath.Join(root, ".work")
	overlayFile := filepath.Join(root, "overlay", "conflict.txt")
	if err := os.MkdirAll(filepath.Dir(overlayFile), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(overlayFile, []byte("overlay"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "patches"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "patches", "compose-unpacker.patch"), []byte("placeholder"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	currentManifest := validManifest()
	composeDir := filepath.Join(workDir, "upstream", "compose-unpacker")
	portainerDir := filepath.Join(workDir, "upstream", "portainer")

	runner := newFakeRunner()
	runner.setOutput(composeDir, "git rev-parse HEAD", currentManifest.Portainer.ComposeUnpackerCommit+"\n")
	runner.setOutput(portainerDir, "git rev-parse HEAD", currentManifest.Portainer.ServerCommit+"\n")
	runner.afterRun(composeDir, "git checkout --detach "+currentManifest.Portainer.ComposeUnpackerCommit, func() error {
		return os.WriteFile(filepath.Join(composeDir, "conflict.txt"), []byte("upstream"), 0o644)
	})

	err := Run(context.Background(), Options{
		Root:     root,
		WorkDir:  workDir,
		Manifest: currentManifest,
	}, runner)
	if err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "copy overlay") {
		t.Fatalf("Run() error = %q, want copy overlay stage", err.Error())
	}
	if !strings.Contains(err.Error(), "conflict.txt") {
		t.Fatalf("Run() error = %q, want conflicting path", err.Error())
	}
}

func TestRunReportsFailingStageAndStderr(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workDir := filepath.Join(root, ".work")
	if err := os.MkdirAll(filepath.Join(root, "patches"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	patchPath := filepath.Join(root, "patches", "compose-unpacker.patch")
	if err := os.WriteFile(patchPath, []byte("placeholder"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	currentManifest := validManifest()
	composeDir := filepath.Join(workDir, "upstream", "compose-unpacker")
	portainerDir := filepath.Join(workDir, "upstream", "portainer")

	runner := newFakeRunner()
	runner.setOutput(composeDir, "git rev-parse HEAD", currentManifest.Portainer.ComposeUnpackerCommit+"\n")
	runner.setOutput(portainerDir, "git rev-parse HEAD", currentManifest.Portainer.ServerCommit+"\n")
	runner.setError(composeDir, "git apply --check "+patchPath, fmt.Errorf("git apply --check failed: stderr: patch rejected"))

	err := Run(context.Background(), Options{
		Root:     root,
		WorkDir:  workDir,
		Manifest: currentManifest,
	}, runner)
	if err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "apply patch") {
		t.Fatalf("Run() error = %q, want apply patch stage", err.Error())
	}
	if !strings.Contains(err.Error(), "patch rejected") {
		t.Fatalf("Run() error = %q, want stderr", err.Error())
	}
}

func TestRunRejectsEmptyWorkDir(t *testing.T) {
	t.Parallel()

	err := Run(context.Background(), Options{
		Root:     t.TempDir(),
		WorkDir:  "",
		Manifest: validManifest(),
	}, newFakeRunner())
	if err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "work directory") {
		t.Fatalf("Run() error = %q, want work directory error", err.Error())
	}
}

type fakeRunner struct {
	runCalls    []fakeCall
	outputCalls []fakeCall
	outputs     map[fakeKey][]byte
	errs        map[fakeKey]error
	after       map[fakeKey]func() error
}

type fakeCall struct {
	dir     string
	command string
}

type fakeKey struct {
	dir     string
	command string
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{
		outputs: make(map[fakeKey][]byte),
		errs:    make(map[fakeKey]error),
		after:   make(map[fakeKey]func() error),
	}
}

func (f *fakeRunner) Run(_ context.Context, dir, name string, args ...string) error {
	command := joinCommand(name, args...)
	f.runCalls = append(f.runCalls, fakeCall{dir: dir, command: command})

	key := fakeKey{dir: dir, command: command}
	if err, ok := f.errs[key]; ok {
		return err
	}
	if fn, ok := f.after[key]; ok {
		return fn()
	}

	return nil
}

func (f *fakeRunner) Output(_ context.Context, dir, name string, args ...string) ([]byte, error) {
	command := joinCommand(name, args...)
	f.outputCalls = append(f.outputCalls, fakeCall{dir: dir, command: command})

	key := fakeKey{dir: dir, command: command}
	if err, ok := f.errs[key]; ok {
		return nil, err
	}
	if output, ok := f.outputs[key]; ok {
		return output, nil
	}

	return nil, fmt.Errorf("unexpected output command %q in %q", command, dir)
}

func (f *fakeRunner) setOutput(dir, command, output string) {
	f.outputs[fakeKey{dir: dir, command: command}] = []byte(output)
}

func (f *fakeRunner) setError(dir, command string, err error) {
	f.errs[fakeKey{dir: dir, command: command}] = err
}

func (f *fakeRunner) afterRun(dir, command string, fn func() error) {
	f.after[fakeKey{dir: dir, command: command}] = fn
}

func (f *fakeRunner) runCommands() []string {
	commands := make([]string, 0, len(f.runCalls))
	for _, call := range f.runCalls {
		commands = append(commands, call.command)
	}
	return commands
}

func (f *fakeRunner) outputCommands() []string {
	commands := make([]string, 0, len(f.outputCalls))
	for _, call := range f.outputCalls {
		commands = append(commands, call.command)
	}
	return commands
}

func joinCommand(name string, args ...string) string {
	if len(args) == 0 {
		return name
	}
	return name + " " + strings.Join(args, " ")
}

func validManifest() manifest.Manifest {
	return manifest.Manifest{
		Portainer: manifest.Portainer{
			Version:               "2.45.0",
			ComposeUnpackerCommit: "23c8e42176c521cb6745b3ea95233d3a68bbe031",
			ServerCommit:          "d79ba726cd54395a54cca5e9180609ce52fa7a4f",
			Image:                 "docker.io/portainer/compose-unpacker",
			LinuxAMD64Digest:      "sha256:25aea494af4f4f04ce46f9cf4c72e49ed21085cc80e63561cc75292da54bd60a",
		},
		Build: manifest.Build{
			GoVersion:           "1.26.6",
			GolangCILintVersion: "v2.13.2",
		},
		SOPS: manifest.SOPS{
			Version: "v3.13.3",
			Asset:   "sops-v3.13.3.linux.amd64",
			URL:     "https://github.com/getsops/sops/releases/download/v3.13.3/sops-v3.13.3.linux.amd64",
			SHA256:  "e5bec3346a873ae91d871550f3e698c1aad962aff462a080e40f25fde17fef6b",
		},
		Platform:        "linux/amd64",
		OverlayRevision: 1,
	}
}
