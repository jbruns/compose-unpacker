package update

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jbruns/compose-unpacker-sops/internal/manifest"
)

const (
	oldServerCommit   = "1111111111111111111111111111111111111111"
	oldUnpackerCommit = "2222222222222222222222222222222222222222"
	newServerCommit   = "3333333333333333333333333333333333333333"
	newUnpackerCommit = "4444444444444444444444444444444444444444"
	oldDigest         = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	newDigest         = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	oldSOPSSHA        = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	newSOPSSHA        = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	portainerImage    = "docker.io/portainer/compose-unpacker"
	oldSOPSAsset      = "sops-v3.13.3.linux.amd64"
	oldSOPSURL        = "https://github.com/getsops/sops/releases/download/v3.13.3/sops-v3.13.3.linux.amd64"
	newSOPSAsset      = "sops-v3.14.0.linux.amd64"
	newSOPSURL        = "https://github.com/getsops/sops/releases/download/v3.14.0/sops-v3.14.0.linux.amd64"
)

func TestResolveSelectsHighestNumericLTS(t *testing.T) {
	t.Parallel()

	sources := successfulSources()
	sources.releases = []Release{
		{TagName: "2.39.8", Name: "Release 2.39.8 LTS"},
		{TagName: "2.45.0", Name: "Release 2.45.0 LTS"},
		{TagName: "2.46.0", Name: "Release 2.46.0 STS"},
		{TagName: "2.99.0", Name: "Release 2.99.0 LTS", Draft: true},
		{TagName: "3.0.0", Name: "Release 3.0.0 LTS", Prerelease: true},
		{TagName: "10.0", Name: "Release 10.0 LTS"},
		{TagName: "2.100.0-rc1", Name: "Release 2.100.0 LTS"},
		{TagName: "2.9.99", Name: "Release 2.9.99 LTS"},
	}
	sources.tagCommits = map[string]string{
		"portainer/2.45.0":        newServerCommit,
		"compose-unpacker/2.45.0": newUnpackerCommit,
	}
	sources.digest = newDigest

	got, summary, err := Resolve(context.Background(), currentManifest(), sources)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Portainer.Version != "2.45.0" {
		t.Fatalf("Portainer.Version = %q, want 2.45.0", got.Portainer.Version)
	}
	if got.Portainer.ServerCommit != newServerCommit {
		t.Fatalf("Portainer.ServerCommit = %q", got.Portainer.ServerCommit)
	}
	if got.Portainer.ComposeUnpackerCommit != newUnpackerCommit {
		t.Fatalf("Portainer.ComposeUnpackerCommit = %q", got.Portainer.ComposeUnpackerCommit)
	}
	if got.Portainer.LinuxAMD64Digest != newDigest {
		t.Fatalf("Portainer.LinuxAMD64Digest = %q", got.Portainer.LinuxAMD64Digest)
	}
	if got.OverlayRevision != 1 {
		t.Fatalf("OverlayRevision = %d, want 1", got.OverlayRevision)
	}
	if !summary.Changed || summary.PortainerBefore != "2.39.8" || summary.PortainerAfter != "2.45.0" {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestResolveRequiresWhitespaceDelimitedLTSToken(t *testing.T) {
	t.Parallel()

	tests := []Release{
		{TagName: "2.45.0", Name: "Release 2.45.0"},
		{TagName: "2.45.0", Name: "Release 2.45.0 LTS-like"},
		{TagName: "2.45.0", Name: "Release 2.45.0 lts"},
	}
	sources := successfulSources()
	sources.releases = tests

	_, _, err := Resolve(context.Background(), currentManifest(), sources)
	if err == nil {
		t.Fatal("Resolve() error = nil, want no valid LTS error")
	}
	if !strings.Contains(err.Error(), "no valid Portainer LTS release") {
		t.Fatalf("Resolve() error = %q", err)
	}
}

func TestResolveComparesNumericComponentsWithoutMachineIntegerLimit(t *testing.T) {
	t.Parallel()

	const largest = "184467440737095516160.0.0"
	sources := successfulSources()
	sources.releases = []Release{
		{TagName: "999.0.0", Name: "Release 999.0.0 LTS"},
		{TagName: largest, Name: "Release " + largest + " LTS"},
	}
	sources.tagCommits = map[string]string{
		"portainer/999.0.0":           newServerCommit,
		"compose-unpacker/999.0.0":    newUnpackerCommit,
		"portainer/" + largest:        newServerCommit,
		"compose-unpacker/" + largest: newUnpackerCommit,
	}

	got, _, err := Resolve(context.Background(), currentManifest(), sources)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Portainer.Version != largest {
		t.Fatalf("Portainer.Version = %q, want %q", got.Portainer.Version, largest)
	}
}

func TestResolveRejectsReleaseListWithoutValidLTS(t *testing.T) {
	t.Parallel()

	sources := successfulSources()
	sources.releases = []Release{
		{TagName: "v2.45.0", Name: "Release v2.45.0 LTS"},
		{TagName: "2.45", Name: "Release 2.45 LTS"},
		{TagName: "2.46.0", Name: "Release 2.46.0 STS"},
		{TagName: "2.47.0", Name: "Release 2.47.0 LTS", Draft: true},
		{TagName: "2.48.0", Name: "Release 2.48.0 LTS", Prerelease: true},
	}

	_, _, err := Resolve(context.Background(), currentManifest(), sources)
	if err == nil {
		t.Fatal("Resolve() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "no valid Portainer LTS release") {
		t.Fatalf("Resolve() error = %q", err)
	}
}

func TestResolveSOPSOnlyUpdateIncrementsRevision(t *testing.T) {
	t.Parallel()

	current := currentManifest()
	current.OverlayRevision = 7
	sources := successfulSources()
	sources.sops = SOPSRelease{
		Version: "v3.14.0",
		Asset:   newSOPSAsset,
		URL:     newSOPSURL,
		SHA256:  newSOPSSHA,
	}

	got, summary, err := Resolve(context.Background(), current, sources)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.OverlayRevision != 8 {
		t.Fatalf("OverlayRevision = %d, want 8", got.OverlayRevision)
	}
	if got.SOPS.Version != "v3.14.0" || got.SOPS.Asset != newSOPSAsset ||
		got.SOPS.URL != newSOPSURL || got.SOPS.SHA256 != newSOPSSHA {
		t.Fatalf("SOPS = %#v", got.SOPS)
	}
	wantSummary := ChangeSummary{
		Changed:         true,
		PortainerBefore: "2.39.8",
		PortainerAfter:  "2.39.8",
		SOPSBefore:      "v3.13.3",
		SOPSAfter:       "v3.14.0",
		OverlayRevision: 8,
	}
	if summary != wantSummary {
		t.Fatalf("summary = %#v, want %#v", summary, wantSummary)
	}
}

func TestResolveNoChangeReturnsManifestExactly(t *testing.T) {
	t.Parallel()

	current := currentManifest()
	sources := successfulSources()

	got, summary, err := Resolve(context.Background(), current, sources)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !reflect.DeepEqual(got, current) {
		t.Fatalf("Resolve() manifest changed:\n got %#v\nwant %#v", got, current)
	}
	wantSummary := ChangeSummary{
		PortainerBefore: "2.39.8",
		PortainerAfter:  "2.39.8",
		SOPSBefore:      "v3.13.3",
		SOPSAfter:       "v3.13.3",
		OverlayRevision: 7,
	}
	if summary != wantSummary {
		t.Fatalf("summary = %#v, want %#v", summary, wantSummary)
	}
}

func TestResolveReportsSourceErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*fakeSources)
		message string
	}{
		{
			name: "missing compose-unpacker tag",
			mutate: func(s *fakeSources) {
				s.tagErrs["compose-unpacker/2.39.8"] = errors.New("matching tag not found")
			},
			message: "resolve compose-unpacker tag 2.39.8",
		},
		{
			name: "missing linux amd64 image",
			mutate: func(s *fakeSources) {
				s.digestErr = errors.New("linux/amd64 manifest not found")
			},
			message: "resolve linux/amd64 image digest",
		},
		{
			name: "missing sops checksum",
			mutate: func(s *fakeSources) {
				s.sopsErr = errors.New("matching checksum not found")
			},
			message: "resolve latest SOPS release",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sources := successfulSources()
			tt.mutate(&sources)

			_, _, err := Resolve(context.Background(), currentManifest(), sources)
			if err == nil {
				t.Fatal("Resolve() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.message) {
				t.Fatalf("Resolve() error = %q, want substring %q", err, tt.message)
			}
		})
	}
}

type fakeSources struct {
	releases   []Release
	releaseErr error
	tagCommits map[string]string
	tagErrs    map[string]error
	digest     string
	digestErr  error
	sops       SOPSRelease
	sopsErr    error
}

func (s fakeSources) PortainerReleases(context.Context) ([]Release, error) {
	return s.releases, s.releaseErr
}

func (s fakeSources) TagCommit(_ context.Context, repository, tag string) (string, error) {
	key := repository + "/" + tag
	if err := s.tagErrs[key]; err != nil {
		return "", err
	}
	commit, ok := s.tagCommits[key]
	if !ok {
		return "", errors.New("tag not found")
	}
	return commit, nil
}

func (s fakeSources) LinuxAMD64Digest(context.Context, string, string) (string, error) {
	return s.digest, s.digestErr
}

func (s fakeSources) LatestSOPS(context.Context) (SOPSRelease, error) {
	return s.sops, s.sopsErr
}

func successfulSources() fakeSources {
	return fakeSources{
		releases: []Release{
			{TagName: "2.39.8", Name: "Release 2.39.8 LTS"},
		},
		tagCommits: map[string]string{
			"portainer/2.39.8":        oldServerCommit,
			"compose-unpacker/2.39.8": oldUnpackerCommit,
		},
		tagErrs: make(map[string]error),
		digest:  oldDigest,
		sops: SOPSRelease{
			Version: "v3.13.3",
			Asset:   oldSOPSAsset,
			URL:     oldSOPSURL,
			SHA256:  oldSOPSSHA,
		},
	}
}

func currentManifest() manifest.Manifest {
	return manifest.Manifest{
		Portainer: manifest.Portainer{
			Version:               "2.39.8",
			ComposeUnpackerCommit: oldUnpackerCommit,
			ServerCommit:          oldServerCommit,
			Image:                 portainerImage,
			LinuxAMD64Digest:      oldDigest,
		},
		Build: manifest.Build{
			GoVersion:           "1.26.6",
			GolangCILintVersion: "v2.13.2",
		},
		SOPS: manifest.SOPS{
			Version: "v3.13.3",
			Asset:   oldSOPSAsset,
			URL:     oldSOPSURL,
			SHA256:  oldSOPSSHA,
		},
		Platform:        "linux/amd64",
		OverlayRevision: 7,
	}
}
