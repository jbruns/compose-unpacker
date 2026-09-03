package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/jbruns/compose-unpacker-sops/internal/fetch"
	"github.com/jbruns/compose-unpacker-sops/internal/manifest"
)

func main() {
	manifestPath := flag.String("manifest", "versions.json", "path to manifest")
	output := flag.String("output", ".work/dist/sops", "download destination")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "fetch-sops accepts no positional arguments")
		os.Exit(2)
	}

	current, err := manifest.Load(*manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create output directory: %v\n", err)
		os.Exit(1)
	}

	client := &http.Client{Timeout: 2 * time.Minute}
	if err := fetch.Download(
		context.Background(),
		client,
		current.SOPS.URL,
		*output,
		current.SOPS.SHA256,
	); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
