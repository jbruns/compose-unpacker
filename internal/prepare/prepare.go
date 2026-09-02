package prepare

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jbruns/compose-unpacker-sops/internal/manifest"
)

const (
	composeUnpackerRepository = "https://github.com/portainer/compose-unpacker.git"
	portainerRepository       = "https://github.com/portainer/portainer.git"
)

type Runner interface {
	Run(ctx context.Context, dir, name string, args ...string) error
	Output(ctx context.Context, dir, name string, args ...string) ([]byte, error)
}

type Options struct {
	Root     string
	WorkDir  string
	Manifest manifest.Manifest
}

type ExecRunner struct{}

func Run(ctx context.Context, options Options, runner Runner) error {
	if runner == nil {
		return errors.New("runner must not be nil")
	}
	if strings.TrimSpace(options.WorkDir) == "" {
		return errors.New("work directory must not be empty")
	}

	root, err := filepath.Abs(options.Root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	workDir, err := filepath.Abs(options.WorkDir)
	if err != nil {
		return fmt.Errorf("resolve work directory: %w", err)
	}
	if workDir == "" || filepath.Dir(workDir) == workDir {
		return fmt.Errorf("refuse to remove root work directory %q", workDir)
	}
	if err := os.RemoveAll(workDir); err != nil {
		return fmt.Errorf("remove work directory: %w", err)
	}

	composeDir := filepath.Join(workDir, "upstream", "compose-unpacker")
	if err := fetchTaggedTree(
		ctx,
		runner,
		composeDir,
		composeUnpackerRepository,
		options.Manifest.Portainer.Version,
		options.Manifest.Portainer.ComposeUnpackerCommit,
	); err != nil {
		return fmt.Errorf("prepare compose-unpacker: %w", err)
	}

	portainerDir := filepath.Join(workDir, "upstream", "portainer")
	if err := fetchTaggedTree(
		ctx,
		runner,
		portainerDir,
		portainerRepository,
		options.Manifest.Portainer.Version,
		options.Manifest.Portainer.ServerCommit,
	); err != nil {
		return fmt.Errorf("prepare portainer: %w", err)
	}

	if err := runner.Run(
		ctx,
		composeDir,
		"go",
		"mod",
		"edit",
		"-replace=github.com/portainer/portainer=../portainer",
	); err != nil {
		return fmt.Errorf("replace portainer module: %w", err)
	}

	var copied []string
	overlayDir := filepath.Join(root, "overlay")
	switch info, err := os.Stat(overlayDir); {
	case err == nil:
		if !info.IsDir() {
			return fmt.Errorf("copy overlay: %s is not a directory", overlayDir)
		}
		copied, err = copyOverlay(overlayDir, composeDir)
		if err != nil {
			return fmt.Errorf("copy overlay: %w", err)
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return fmt.Errorf("copy overlay: %w", err)
	}

	patchPath := filepath.Join(root, "patches", "compose-unpacker.patch")
	if err := applyPatch(ctx, runner, composeDir, patchPath); err != nil {
		return fmt.Errorf("apply patch: %w", err)
	}

	allowedComposeChanges := append([]string{"commands/compose_deploy.go", "go.mod"}, copied...)
	if err := verifyChanges(ctx, runner, composeDir, allowedComposeChanges); err != nil {
		return fmt.Errorf("verify compose-unpacker changes: %w", err)
	}
	if err := verifyChanges(ctx, runner, portainerDir, nil); err != nil {
		return fmt.Errorf("verify portainer changes: %w", err)
	}

	return nil
}

func fetchTaggedTree(ctx context.Context, runner Runner, destination, repository, version, commit string) error {
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", destination, err)
	}

	commands := [][]string{
		{"git", "init"},
		{"git", "remote", "add", "origin", repository},
		{"git", "fetch", "--depth=1", "origin", "refs/tags/" + version},
		{"git", "checkout", "--detach", commit},
	}
	for _, command := range commands {
		if err := runner.Run(ctx, destination, command[0], command[1:]...); err != nil {
			return err
		}
	}

	output, err := runner.Output(ctx, destination, "git", "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if got := strings.TrimSpace(string(output)); got != commit {
		return fmt.Errorf("fetched commit %q, want %q", got, commit)
	}

	return nil
}

func copyOverlay(source, destination string) ([]string, error) {
	var copied []string

	if err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}

		target := filepath.Join(destination, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}

		switch {
		case entry.IsDir():
			existing, statErr := os.Stat(target)
			switch {
			case statErr == nil:
				if !existing.IsDir() {
					return fmt.Errorf("%s already exists", target)
				}
				return nil
			case !errors.Is(statErr, os.ErrNotExist):
				return statErr
			}
			return os.MkdirAll(target, info.Mode().Perm())
		case info.Mode().IsRegular():
			if _, statErr := os.Lstat(target); statErr == nil {
				return fmt.Errorf("%s already exists", target)
			} else if !errors.Is(statErr, os.ErrNotExist) {
				return statErr
			}
			if err := copyFile(path, target, info.Mode().Perm()); err != nil {
				return err
			}
			copied = append(copied, relative)
			return nil
		default:
			return fmt.Errorf("unsupported overlay entry %s", path)
		}
	}); err != nil {
		return nil, err
	}

	return copied, nil
}

func applyPatch(ctx context.Context, runner Runner, repository, patch string) error {
	if err := runner.Run(ctx, repository, "git", "apply", "--check", patch); err != nil {
		return err
	}
	return runner.Run(ctx, repository, "git", "apply", patch)
}

func verifyChanges(ctx context.Context, runner Runner, repository string, allowed []string) error {
	output, err := runner.Output(ctx, repository, "git", "status", "--porcelain")
	if err != nil {
		return err
	}

	var actual []string
	for _, line := range strings.Split(strings.TrimRight(string(output), "\n"), "\n") {
		if line == "" {
			continue
		}
		if len(line) < 4 {
			return fmt.Errorf("unexpected git status line %q", line)
		}
		actual = append(actual, line[3:])
	}

	slices.Sort(actual)
	allowed = append([]string(nil), allowed...)
	slices.Sort(allowed)
	if !slices.Equal(actual, allowed) {
		return fmt.Errorf("dirty paths = %v, want %v", actual, allowed)
	}

	return nil
}

func (ExecRunner) Run(ctx context.Context, dir, name string, args ...string) error {
	_, err := runCommand(ctx, dir, name, args...)
	return err
}

func (ExecRunner) Output(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	return runCommand(ctx, dir, name, args...)
}

func runCommand(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		return nil, &commandError{
			name:   name,
			args:   append([]string(nil), args...),
			stderr: strings.TrimSpace(stderr.String()),
			err:    err,
		}
	}

	return stdout.Bytes(), nil
}

func copyFile(source, destination string, mode fs.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open %s: %w", source, err)
	}
	defer input.Close()

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(destination), err)
	}

	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("create %s: %w", destination, err)
	}
	defer output.Close()

	if _, err := io.Copy(output, input); err != nil {
		return fmt.Errorf("copy %s to %s: %w", source, destination, err)
	}

	return nil
}

type commandError struct {
	name   string
	args   []string
	stderr string
	err    error
}

func (e *commandError) Error() string {
	command := e.name
	if len(e.args) > 0 {
		command += " " + strings.Join(e.args, " ")
	}
	if e.stderr == "" {
		return fmt.Sprintf("%s: %v", command, e.err)
	}
	return fmt.Sprintf("%s: %v: %s", command, e.err, e.stderr)
}

func (e *commandError) Unwrap() error {
	return e.err
}
