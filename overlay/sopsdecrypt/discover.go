package sopsdecrypt

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
)

const sopsMarker = ".sops."

type encryptedFile struct {
	Source      string
	Destination string
}

type walkDirFunc func(root string, fn fs.WalkDirFunc) error

func rootDirectories(composeFiles []string) ([]string, error) {
	directories := make([]string, 0, len(composeFiles))
	seen := make(map[string]struct{}, len(composeFiles))
	for _, composeFile := range composeFiles {
		directory := filepath.Dir(filepath.Clean(composeFile))
		if _, exists := seen[directory]; exists {
			continue
		}
		seen[directory] = struct{}{}
		directories = append(directories, directory)
	}
	slices.Sort(directories)

	roots := make([]string, 0, len(directories))
	for _, candidate := range directories {
		contained := false
		for _, root := range roots {
			relative, err := filepath.Rel(root, candidate)
			if err != nil {
				return nil, fmt.Errorf("compare compose directories %q and %q: %w", root, candidate, err)
			}
			if relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
				contained = true
				break
			}
		}
		if !contained {
			roots = append(roots, candidate)
		}
	}

	return roots, nil
}

func discover(composeFiles []string, walk walkDirFunc) ([]encryptedFile, error) {
	if walk == nil {
		return nil, errors.New("walk function must not be nil")
	}

	roots, err := rootDirectories(composeFiles)
	if err != nil {
		return nil, err
	}

	var sources []string
	for _, root := range roots {
		if err := walk(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if entry.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			if !entry.Type().IsRegular() || !strings.Contains(entry.Name(), sopsMarker) {
				return nil
			}
			sources = append(sources, path)
			return nil
		}); err != nil {
			return nil, fmt.Errorf("walk %q: %w", root, err)
		}
	}
	slices.Sort(sources)

	files := make([]encryptedFile, 0, len(sources))
	destinations := make(map[string]string, len(sources))
	for _, source := range sources {
		destination, err := outputPath(source)
		if err != nil {
			return nil, err
		}
		if previous, exists := destinations[destination]; exists {
			return nil, fmt.Errorf("encrypted sources %q and %q both produce destination %q", previous, source, destination)
		}
		destinations[destination] = source
		files = append(files, encryptedFile{Source: source, Destination: destination})
	}

	return files, nil
}

func outputPath(source string) (string, error) {
	base := filepath.Base(source)
	if !strings.Contains(base, sopsMarker) {
		return "", fmt.Errorf("encrypted source %q does not contain %q", source, sopsMarker)
	}
	// The replacement is a separator-free base name from the walked source.
	return filepath.Join(filepath.Dir(source), strings.Replace(base, sopsMarker, ".", 1)), nil //nolint:forbidigo
}
