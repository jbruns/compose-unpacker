package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/jbruns/compose-unpacker-sops/internal/manifest"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("manifest-value", flag.ContinueOnError)
	flags.SetOutput(stderr)

	manifestPath := flags.String("manifest", "versions.json", "path to manifest")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "expected exactly one field")
		return 2
	}

	value, err := lookup(*manifestPath, flags.Arg(0))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if _, err := fmt.Fprintln(stdout, value); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	return 0
}

func lookup(path, field string) (string, error) {
	current, err := manifest.Load(path)
	if err != nil {
		return "", err
	}

	switch field {
	case "go-version":
		return current.Build.GoVersion, nil
	case "lint-version":
		return current.Build.GolangCILintVersion, nil
	case "base-image":
		return current.BaseImage(), nil
	case "base-digest":
		return current.Portainer.LinuxAMD64Digest, nil
	case "portainer-version":
		return current.Portainer.Version, nil
	case "sops-version":
		return current.SOPS.Version, nil
	case "overlay-revision":
		return strconv.Itoa(current.OverlayRevision), nil
	case "immutable-tag":
		return current.ImmutableTag(), nil
	case "version-tag":
		return current.VersionTag(), nil
	default:
		return "", fmt.Errorf("unknown field %q", field)
	}
}
