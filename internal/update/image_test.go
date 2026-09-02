package update

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const imageDigest = "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"

func TestLinuxAMD64DigestReturnsOnlyMatchingChildManifest(t *testing.T) {
	t.Parallel()

	inspector := staticInspector{output: []byte(`{
		"schemaVersion": 2,
		"mediaType": "application/vnd.oci.image.index.v1+json",
		"manifests": [
			{
				"mediaType": "application/vnd.oci.image.manifest.v1+json",
				"digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"platform": {"os": "linux", "architecture": "arm64"}
			},
			{
				"mediaType": "application/vnd.oci.image.manifest.v1+json",
				"digest": "` + imageDigest + `",
				"platform": {"os": "linux", "architecture": "amd64"}
			}
		]
	}`)}

	got, err := LinuxAMD64Digest(context.Background(), inspector, "docker.io/portainer/compose-unpacker", "2.45.0")
	if err != nil {
		t.Fatalf("LinuxAMD64Digest() error = %v", err)
	}
	if got != imageDigest {
		t.Fatalf("LinuxAMD64Digest() = %q, want %q", got, imageDigest)
	}
}

func TestLinuxAMD64DigestRejectsInvalidIndexes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "malformed json",
			output: `{"schemaVersion":`,
			want:   "parse image index",
		},
		{
			name: "wrong media type",
			output: `{
				"schemaVersion": 2,
				"mediaType": "application/vnd.oci.image.manifest.v1+json",
				"manifests": []
			}`,
			want: "OCI image index",
		},
		{
			name: "absent linux amd64 manifest",
			output: `{
				"schemaVersion": 2,
				"mediaType": "application/vnd.oci.image.index.v1+json",
				"manifests": [{
					"digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					"platform": {"os": "linux", "architecture": "arm64"}
				}]
			}`,
			want: "exactly one linux/amd64",
		},
		{
			name: "duplicate linux amd64 manifests",
			output: `{
				"schemaVersion": 2,
				"mediaType": "application/vnd.oci.image.index.v1+json",
				"manifests": [
					{
						"digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
						"platform": {"os": "linux", "architecture": "amd64"}
					},
					{
						"digest": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
						"platform": {"os": "linux", "architecture": "amd64"}
					}
				]
			}`,
			want: "exactly one linux/amd64",
		},
		{
			name: "malformed matching digest",
			output: `{
				"schemaVersion": 2,
				"mediaType": "application/vnd.oci.image.index.v1+json",
				"manifests": [{
					"digest": "sha256:not-a-digest",
					"platform": {"os": "linux", "architecture": "amd64"}
				}]
			}`,
			want: "invalid sha256 digest",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := LinuxAMD64Digest(
				context.Background(),
				staticInspector{output: []byte(tt.output)},
				"docker.io/portainer/compose-unpacker",
				"2.45.0",
			)
			if err == nil {
				t.Fatal("LinuxAMD64Digest() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LinuxAMD64Digest() error = %q, want substring %q", err, tt.want)
			}
		})
	}
}

func TestLinuxAMD64DigestReportsInspectionFailure(t *testing.T) {
	t.Parallel()

	_, err := LinuxAMD64Digest(
		context.Background(),
		staticInspector{err: errors.New("inspect failed")},
		"docker.io/portainer/compose-unpacker",
		"2.45.0",
	)
	if err == nil {
		t.Fatal("LinuxAMD64Digest() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "inspect image") {
		t.Fatalf("LinuxAMD64Digest() error = %q", err)
	}
}

type staticInspector struct {
	output []byte
	err    error
}

func (s staticInspector) Inspect(context.Context, string) ([]byte, error) {
	return s.output, s.err
}
