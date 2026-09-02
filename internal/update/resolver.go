package update

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/jbruns/compose-unpacker-sops/internal/manifest"
)

var threePartVersion = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`)

type Release struct {
	TagName    string
	Name       string
	Draft      bool
	Prerelease bool
}

type SOPSRelease struct {
	Version string
	Asset   string
	URL     string
	SHA256  string
}

type ChangeSummary struct {
	Changed         bool   `json:"changed"`
	PortainerBefore string `json:"portainerBefore"`
	PortainerAfter  string `json:"portainerAfter"`
	SOPSBefore      string `json:"sopsBefore"`
	SOPSAfter       string `json:"sopsAfter"`
	OverlayRevision int    `json:"overlayRevision"`
}

type Sources interface {
	PortainerReleases(context.Context) ([]Release, error)
	TagCommit(context.Context, string, string) (string, error)
	LinuxAMD64Digest(context.Context, string, string) (string, error)
	LatestSOPS(context.Context) (SOPSRelease, error)
}

func Resolve(ctx context.Context, current manifest.Manifest, sources Sources) (manifest.Manifest, ChangeSummary, error) {
	releases, err := sources.PortainerReleases(ctx)
	if err != nil {
		return manifest.Manifest{}, ChangeSummary{}, fmt.Errorf("resolve Portainer releases: %w", err)
	}

	portainerRelease, err := highestLTS(releases)
	if err != nil {
		return manifest.Manifest{}, ChangeSummary{}, err
	}

	serverCommit, err := sources.TagCommit(ctx, "portainer", portainerRelease.TagName)
	if err != nil {
		return manifest.Manifest{}, ChangeSummary{}, fmt.Errorf("resolve portainer tag %s: %w", portainerRelease.TagName, err)
	}
	unpackerCommit, err := sources.TagCommit(ctx, "compose-unpacker", portainerRelease.TagName)
	if err != nil {
		return manifest.Manifest{}, ChangeSummary{}, fmt.Errorf("resolve compose-unpacker tag %s: %w", portainerRelease.TagName, err)
	}
	digest, err := sources.LinuxAMD64Digest(ctx, current.Portainer.Image, portainerRelease.TagName)
	if err != nil {
		return manifest.Manifest{}, ChangeSummary{}, fmt.Errorf("resolve linux/amd64 image digest: %w", err)
	}
	sops, err := sources.LatestSOPS(ctx)
	if err != nil {
		return manifest.Manifest{}, ChangeSummary{}, fmt.Errorf("resolve latest SOPS release: %w", err)
	}

	next := current
	next.Portainer.Version = portainerRelease.TagName
	next.Portainer.ServerCommit = serverCommit
	next.Portainer.ComposeUnpackerCommit = unpackerCommit
	next.Portainer.LinuxAMD64Digest = digest
	next.SOPS = manifest.SOPS{
		Version: sops.Version,
		Asset:   sops.Asset,
		URL:     sops.URL,
		SHA256:  sops.SHA256,
	}

	portainerChanged := next.Portainer != current.Portainer
	sopsChanged := next.SOPS != current.SOPS
	switch {
	case portainerChanged:
		next.OverlayRevision = 1
	case sopsChanged:
		next.OverlayRevision++
	}

	if err := next.Validate(); err != nil {
		return manifest.Manifest{}, ChangeSummary{}, fmt.Errorf("validate resolved manifest: %w", err)
	}

	summary := ChangeSummary{
		Changed:         next != current,
		PortainerBefore: current.Portainer.Version,
		PortainerAfter:  next.Portainer.Version,
		SOPSBefore:      current.SOPS.Version,
		SOPSAfter:       next.SOPS.Version,
		OverlayRevision: next.OverlayRevision,
	}
	return next, summary, nil
}

func highestLTS(releases []Release) (Release, error) {
	var selected Release
	var selectedParts [3]int
	found := false

	for _, release := range releases {
		if release.Draft || release.Prerelease || !hasToken(release.Name, "LTS") {
			continue
		}

		matches := threePartVersion.FindStringSubmatch(release.TagName)
		if matches == nil {
			continue
		}

		var parts [3]int
		valid := true
		for i := range parts {
			part, err := strconv.Atoi(matches[i+1])
			if err != nil {
				valid = false
				break
			}
			parts[i] = part
		}
		if !valid {
			continue
		}

		if !found || versionGreater(parts, selectedParts) {
			selected = release
			selectedParts = parts
			found = true
		}
	}

	if !found {
		return Release{}, fmt.Errorf("no valid Portainer LTS release")
	}
	return selected, nil
}

func hasToken(value, token string) bool {
	for _, field := range strings.Fields(value) {
		if field == token {
			return true
		}
	}
	return false
}

func versionGreater(left, right [3]int) bool {
	for i := range left {
		if left[i] != right[i] {
			return left[i] > right[i]
		}
	}
	return false
}
