package manifest_test

import (
	"strings"
	"testing"

	manifest "github.com/jbruns/compose-unpacker-sops/internal/manifest"
)

func TestManifestValidate(t *testing.T) {
	t.Parallel()

	valid := validManifest()

	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if got := valid.ImmutableTag(); got != "2.45.0-sops.1" {
		t.Fatalf("ImmutableTag() = %q", got)
	}
	if got := valid.VersionTag(); got != "2.45.0-sops" {
		t.Fatalf("VersionTag() = %q", got)
	}
	if got := valid.BaseImage(); got != valid.Portainer.Image+"@"+valid.Portainer.LinuxAMD64Digest {
		t.Fatalf("BaseImage() = %q", got)
	}
}

func TestManifestValidateRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*manifest.Manifest)
		want   string
	}{
		{
			name: "bad portainer version",
			mutate: func(m *manifest.Manifest) {
				m.Portainer.Version = "v2.45.0"
			},
			want: "portainer.version",
		},
		{
			name: "bad compose unpacker commit",
			mutate: func(m *manifest.Manifest) {
				m.Portainer.ComposeUnpackerCommit = "23C8E42176C521CB6745B3EA95233D3A68BBE031"
			},
			want: "portainer.composeUnpackerCommit",
		},
		{
			name: "bad server commit",
			mutate: func(m *manifest.Manifest) {
				m.Portainer.ServerCommit = "not-a-commit"
			},
			want: "portainer.serverCommit",
		},
		{
			name: "bad image digest",
			mutate: func(m *manifest.Manifest) {
				m.Portainer.LinuxAMD64Digest = "sha256:XYZ"
			},
			want: "portainer.linuxAMD64Digest",
		},
		{
			name: "bad go version",
			mutate: func(m *manifest.Manifest) {
				m.Build.GoVersion = "v1.26.6"
			},
			want: "build.goVersion",
		},
		{
			name: "bad lint version",
			mutate: func(m *manifest.Manifest) {
				m.Build.GolangCILintVersion = "2.13.2"
			},
			want: "build.golangciLintVersion",
		},
		{
			name: "bad sops version",
			mutate: func(m *manifest.Manifest) {
				m.SOPS.Version = "3.13.3"
			},
			want: "sops.version",
		},
		{
			name: "bad sops asset",
			mutate: func(m *manifest.Manifest) {
				m.SOPS.Asset = "sops-v3.13.4.linux.amd64"
			},
			want: "sops.asset",
		},
		{
			name: "bad sops url",
			mutate: func(m *manifest.Manifest) {
				m.SOPS.URL = "https://example.com/sops"
			},
			want: "sops.url",
		},
		{
			name: "bad sops checksum",
			mutate: func(m *manifest.Manifest) {
				m.SOPS.SHA256 = "E5BEC3346A873AE91D871550F3E698C1AAD962AFF462A080E40F25FDE17FEF6B"
			},
			want: "sops.sha256",
		},
		{
			name: "unsupported platform",
			mutate: func(m *manifest.Manifest) {
				m.Platform = "linux/arm64"
			},
			want: "platform",
		},
		{
			name: "non-positive overlay revision",
			mutate: func(m *manifest.Manifest) {
				m.OverlayRevision = 0
			},
			want: "overlayRevision",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := validManifest()
			tt.mutate(&got)

			err := got.Validate()
			if err == nil {
				t.Fatal("Validate() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %q, want substring %q", err.Error(), tt.want)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	t.Parallel()

	got, err := manifest.Load("../../versions.json")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got != validManifest() {
		t.Fatalf("Load() = %#v, want %#v", got, validManifest())
	}
}

func TestLoadRejectsInvalidManifests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "unknown field",
			path: "testdata/unknown-field.json",
			want: "decode testdata/unknown-field.json",
		},
		{
			name: "trailing json",
			path: "testdata/trailing.json",
			want: "decode testdata/trailing.json: trailing JSON after top-level object",
		},
		{
			name: "validation",
			path: "testdata/invalid-validation.json",
			want: "validate testdata/invalid-validation.json: overlayRevision",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := manifest.Load(tt.path)
			if err == nil {
				t.Fatal("Load() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load() error = %q, want substring %q", err.Error(), tt.want)
			}
		})
	}
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
