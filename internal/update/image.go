package update

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
)

const ociIndexMediaType = "application/vnd.oci.image.index.v1+json"

var imageDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type DockerImageInspector struct{}

func (DockerImageInspector) Inspect(ctx context.Context, image string) ([]byte, error) {
	command := exec.CommandContext(
		ctx,
		"docker",
		"buildx",
		"imagetools",
		"inspect",
		"--format",
		"{{json .Manifest}}",
		image,
	)
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("docker buildx imagetools inspect: %w", err)
	}
	return output, nil
}

func LinuxAMD64Digest(ctx context.Context, inspector ImageInspector, image, tag string) (string, error) {
	if inspector == nil {
		return "", fmt.Errorf("image inspector must not be nil")
	}
	if image == "" || tag == "" {
		return "", fmt.Errorf("image and tag must not be empty")
	}

	output, err := inspector.Inspect(ctx, image+":"+tag)
	if err != nil {
		return "", fmt.Errorf("inspect image: %w", err)
	}

	var index struct {
		SchemaVersion int    `json:"schemaVersion"`
		MediaType     string `json:"mediaType"`
		Manifests     []struct {
			Digest   string `json:"digest"`
			Platform struct {
				OS           string `json:"os"`
				Architecture string `json:"architecture"`
			} `json:"platform"`
		} `json:"manifests"`
	}
	if err := json.Unmarshal(output, &index); err != nil {
		return "", fmt.Errorf("parse image index")
	}
	if index.SchemaVersion != 2 || index.MediaType != ociIndexMediaType {
		return "", fmt.Errorf("image is not an OCI image index")
	}

	var matches []string
	for _, child := range index.Manifests {
		if child.Platform.OS == "linux" && child.Platform.Architecture == "amd64" {
			matches = append(matches, child.Digest)
		}
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("image index must contain exactly one linux/amd64 child manifest")
	}
	if !imageDigestPattern.MatchString(matches[0]) {
		return "", fmt.Errorf("linux/amd64 child manifest has invalid sha256 digest")
	}
	return matches[0], nil
}

func (c *GitHubClient) LinuxAMD64Digest(ctx context.Context, image, tag string) (string, error) {
	return LinuxAMD64Digest(ctx, c.inspector, image, tag)
}
