package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/jbruns/compose-unpacker-sops/internal/manifest"
	"github.com/jbruns/compose-unpacker-sops/internal/prepare"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	_ = stdout

	flags := flag.NewFlagSet("prepare", flag.ContinueOnError)
	flags.SetOutput(stderr)

	manifestPath := flags.String("manifest", "versions.json", "path to manifest")
	workDir := flags.String("work-dir", ".work", "working directory")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "prepare accepts no positional arguments")
		return 2
	}

	currentManifest, err := manifest.Load(*manifestPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if err := prepare.Run(context.Background(), prepare.Options{
		Root:     root,
		WorkDir:  *workDir,
		Manifest: currentManifest,
	}, prepare.ExecRunner{}); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	return 0
}
