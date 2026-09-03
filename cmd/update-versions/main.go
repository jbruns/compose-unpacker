package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/jbruns/compose-unpacker-sops/internal/manifest"
	"github.com/jbruns/compose-unpacker-sops/internal/update"
)

const gitHubAPIURL = "https://api.github.com"

type Sources = update.Sources

func main() {
	httpClient := &http.Client{Timeout: 30 * time.Second}
	sources := update.NewGitHubClient(
		httpClient,
		gitHubAPIURL,
		os.Getenv("GITHUB_TOKEN"),
		update.DockerImageInspector{},
	)
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, sources))
}

func run(args []string, stdout, stderr io.Writer, sources Sources) int {
	return runWithRename(args, stdout, stderr, sources, os.Rename)
}

func runWithRename(
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	sources Sources,
	rename func(string, string) error,
) int {
	flags := flag.NewFlagSet("update-versions", flag.ContinueOnError)
	flags.SetOutput(stderr)
	manifestPath := flags.String("manifest", "versions.json", "path to versions manifest")
	check := flags.Bool("check", false, "report whether an update is available")
	write := flags.Bool("write", false, "atomically write available updates")

	if err := flags.Parse(args); err != nil {
		return 1
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "unexpected positional arguments")
		return 1
	}
	if *check == *write {
		fmt.Fprintln(stderr, "exactly one of -check or -write is required")
		return 1
	}

	current, err := manifest.Load(*manifestPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	next, summary, err := update.Resolve(context.Background(), current, sources)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if *write && summary.Changed {
		contents, err := json.MarshalIndent(next, "", "  ")
		if err != nil {
			fmt.Fprintln(stderr, "encode manifest")
			return 1
		}
		contents = append(contents, '\n')
		if err := atomicWrite(*manifestPath, contents, rename); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}

	if err := json.NewEncoder(stdout).Encode(summary); err != nil {
		fmt.Fprintln(stderr, "write change summary")
		return 1
	}
	if *check && summary.Changed {
		return 2
	}
	return 0
}

func atomicWrite(path string, contents []byte, rename func(string, string) error) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat manifest: %w", err)
	}

	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary manifest: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		temporary.Close()
		if temporaryPath != "" {
			os.Remove(temporaryPath)
		}
	}()

	if err := temporary.Chmod(info.Mode().Perm()); err != nil {
		return fmt.Errorf("set temporary manifest permissions: %w", err)
	}
	if _, err := temporary.Write(contents); err != nil {
		return fmt.Errorf("write temporary manifest: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary manifest: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary manifest: %w", err)
	}
	if err := rename(temporaryPath, path); err != nil {
		return fmt.Errorf("rename manifest: %w", err)
	}
	temporaryPath = ""
	return nil
}
