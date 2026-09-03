package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
)

const linuxAMD64 = "linux/amd64"

var (
	plainVersionPattern  = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	prefixedVersionRegex = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
	sha1Pattern          = regexp.MustCompile(`^[0-9a-f]{40}$`)
	sha256Pattern        = regexp.MustCompile(`^[0-9a-f]{64}$`)
	digestPattern        = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

type Manifest struct {
	Portainer       Portainer `json:"portainer"`
	Build           Build     `json:"build"`
	SOPS            SOPS      `json:"sops"`
	Platform        string    `json:"platform"`
	OverlayRevision int       `json:"overlayRevision"`
}

type Portainer struct {
	Version               string `json:"version"`
	ComposeUnpackerCommit string `json:"composeUnpackerCommit"`
	ServerCommit          string `json:"serverCommit"`
	Image                 string `json:"image"`
	LinuxAMD64Digest      string `json:"linuxAMD64Digest"`
}

type Build struct {
	GoVersion           string `json:"goVersion"`
	GolangCILintVersion string `json:"golangciLintVersion"`
}

type SOPS struct {
	Version string `json:"version"`
	Asset   string `json:"asset"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
}

func Load(path string) (Manifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()

	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode %s: %w", path, err)
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Manifest{}, fmt.Errorf("decode %s: %w", path, errors.New("trailing JSON after top-level object"))
		}
		return Manifest{}, fmt.Errorf("decode %s: %w", path, err)
	}

	if err := manifest.Validate(); err != nil {
		return Manifest{}, fmt.Errorf("validate %s: %w", path, err)
	}

	return manifest, nil
}

func (m Manifest) Validate() error {
	if !plainVersionPattern.MatchString(m.Portainer.Version) {
		return fmt.Errorf("portainer.version must match %q", plainVersionPattern.String())
	}
	if !sha1Pattern.MatchString(m.Portainer.ComposeUnpackerCommit) {
		return fmt.Errorf("portainer.composeUnpackerCommit must be 40 lowercase hex characters")
	}
	if !sha1Pattern.MatchString(m.Portainer.ServerCommit) {
		return fmt.Errorf("portainer.serverCommit must be 40 lowercase hex characters")
	}
	if m.Portainer.Image == "" {
		return fmt.Errorf("portainer.image must not be empty")
	}
	if !digestPattern.MatchString(m.Portainer.LinuxAMD64Digest) {
		return fmt.Errorf("portainer.linuxAMD64Digest must be a sha256 digest")
	}
	if !plainVersionPattern.MatchString(m.Build.GoVersion) {
		return fmt.Errorf("build.goVersion must match %q", plainVersionPattern.String())
	}
	if !prefixedVersionRegex.MatchString(m.Build.GolangCILintVersion) {
		return fmt.Errorf("build.golangciLintVersion must match %q", prefixedVersionRegex.String())
	}
	if !prefixedVersionRegex.MatchString(m.SOPS.Version) {
		return fmt.Errorf("sops.version must match %q", prefixedVersionRegex.String())
	}

	expectedAsset := fmt.Sprintf("sops-%s.linux.amd64", m.SOPS.Version)
	if m.SOPS.Asset != expectedAsset {
		return fmt.Errorf("sops.asset must equal %q", expectedAsset)
	}

	expectedURL := fmt.Sprintf("https://github.com/getsops/sops/releases/download/%s/%s", m.SOPS.Version, m.SOPS.Asset)
	if m.SOPS.URL != expectedURL {
		return fmt.Errorf("sops.url must equal %q", expectedURL)
	}
	if !sha256Pattern.MatchString(m.SOPS.SHA256) {
		return fmt.Errorf("sops.sha256 must be 64 lowercase hex characters")
	}
	if m.Platform != linuxAMD64 {
		return fmt.Errorf("platform must equal %q", linuxAMD64)
	}
	if m.OverlayRevision <= 0 {
		return fmt.Errorf("overlayRevision must be positive")
	}

	return nil
}

func (m Manifest) ImmutableTag() string {
	return fmt.Sprintf("%s.%d", m.VersionTag(), m.OverlayRevision)
}

func (m Manifest) VersionTag() string {
	return fmt.Sprintf("%s-sops", m.Portainer.Version)
}

func (m Manifest) ReleaseTags(repository string) []string {
	return []string{
		fmt.Sprintf("%s:%s", repository, m.ImmutableTag()),
		fmt.Sprintf("%s:%s", repository, m.VersionTag()),
		fmt.Sprintf("%s:lts-sops", repository),
	}
}

func (m Manifest) BaseImage() string {
	return fmt.Sprintf("%s@%s", m.Portainer.Image, m.Portainer.LinuxAMD64Digest)
}
