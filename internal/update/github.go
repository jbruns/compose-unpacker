package update

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const (
	gitHubAccept     = "application/vnd.github+json"
	gitHubAPIVersion = "2022-11-28"
	maxResponseBytes = 10 << 20
)

var (
	commitPattern      = regexp.MustCompile(`^[0-9a-f]{40}$`)
	sopsVersionPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
	checksumPattern    = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type ImageInspector interface {
	Inspect(context.Context, string) ([]byte, error)
}

type GitHubClient struct {
	httpClient *http.Client
	baseURL    string
	token      string
	inspector  ImageInspector
}

func NewGitHubClient(httpClient *http.Client, baseURL, token string, inspector ImageInspector) *GitHubClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &GitHubClient{
		httpClient: httpClient,
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		inspector:  inspector,
	}
}

func (c *GitHubClient) PortainerReleases(ctx context.Context) ([]Release, error) {
	next := c.baseURL + "/repos/portainer/portainer/releases"
	var releases []Release

	for page := 0; next != ""; page++ {
		if page == 100 {
			return nil, fmt.Errorf("list Portainer releases: too many response pages")
		}

		var response []struct {
			TagName    string `json:"tag_name"`
			Name       string `json:"name"`
			Draft      bool   `json:"draft"`
			Prerelease bool   `json:"prerelease"`
		}
		headers, err := c.getJSON(ctx, next, &response)
		if err != nil {
			return nil, fmt.Errorf("list Portainer releases: %w", err)
		}
		for _, item := range response {
			releases = append(releases, Release{
				TagName:    item.TagName,
				Name:       item.Name,
				Draft:      item.Draft,
				Prerelease: item.Prerelease,
			})
		}
		next = nextLink(headers.Get("Link"))
	}

	return releases, nil
}

func (c *GitHubClient) TagCommit(ctx context.Context, repository, tag string) (string, error) {
	if repository == "" || tag == "" {
		return "", fmt.Errorf("repository and tag must not be empty")
	}

	var object gitObject
	refURL := fmt.Sprintf("%s/repos/portainer/%s/git/ref/tags/%s",
		c.baseURL, url.PathEscape(repository), url.PathEscape(tag))
	if _, err := c.getJSON(ctx, refURL, &struct {
		Object *gitObject `json:"object"`
	}{Object: &object}); err != nil {
		return "", fmt.Errorf("get tag ref: %w", err)
	}

	for depth := 0; depth < 10; depth++ {
		switch object.Type {
		case "commit":
			if !commitPattern.MatchString(object.SHA) {
				return "", fmt.Errorf("tag commit has invalid SHA")
			}
			return object.SHA, nil
		case "tag":
			var tagObject gitObject
			tagURL := fmt.Sprintf("%s/repos/portainer/%s/git/tags/%s",
				c.baseURL, url.PathEscape(repository), url.PathEscape(object.SHA))
			if _, err := c.getJSON(ctx, tagURL, &struct {
				Object *gitObject `json:"object"`
			}{Object: &tagObject}); err != nil {
				return "", fmt.Errorf("dereference annotated tag: %w", err)
			}
			object = tagObject
		default:
			return "", fmt.Errorf("tag does not resolve to a commit")
		}
	}

	return "", fmt.Errorf("tag does not resolve to a commit: too many annotated tag levels")
}

func (c *GitHubClient) LatestSOPS(ctx context.Context) (SOPSRelease, error) {
	var response struct {
		TagName    string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if _, err := c.getJSON(ctx, c.baseURL+"/repos/getsops/sops/releases/latest", &response); err != nil {
		return SOPSRelease{}, fmt.Errorf("get latest SOPS release: %w", err)
	}
	if response.Draft || response.Prerelease || !sopsVersionPattern.MatchString(response.TagName) {
		return SOPSRelease{}, fmt.Errorf("latest SOPS release is not a stable three-part version")
	}

	assetName := "sops-" + response.TagName + ".linux.amd64"
	checksumName := "sops-" + response.TagName + ".checksums.txt"
	assetURLs := exactAssetURLs(response.Assets, assetName)
	checksumURLs := exactAssetURLs(response.Assets, checksumName)
	if len(assetURLs) != 1 {
		return SOPSRelease{}, fmt.Errorf("latest SOPS release must contain exactly one asset named %q", assetName)
	}
	if len(checksumURLs) != 1 {
		return SOPSRelease{}, fmt.Errorf("latest SOPS release must contain exactly one asset named %q", checksumName)
	}
	if assetURLs[0] == "" || checksumURLs[0] == "" {
		return SOPSRelease{}, fmt.Errorf("latest SOPS release asset URL must not be empty")
	}

	checksums, err := c.getBytes(ctx, checksumURLs[0])
	if err != nil {
		return SOPSRelease{}, fmt.Errorf("get SOPS checksums: %w", err)
	}
	checksum, err := exactChecksum(checksums, assetName)
	if err != nil {
		return SOPSRelease{}, err
	}

	return SOPSRelease{
		Version: response.TagName,
		Asset:   assetName,
		URL:     assetURLs[0],
		SHA256:  checksum,
	}, nil
}

type gitObject struct {
	Type string `json:"type"`
	SHA  string `json:"sha"`
}

func exactAssetURLs(assets []struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}, name string) []string {
	var matches []string
	for _, asset := range assets {
		if asset.Name == name {
			matches = append(matches, asset.URL)
		}
	}
	return matches
}

func exactChecksum(contents []byte, assetName string) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(contents))
	scanner.Buffer(make([]byte, 64*1024), maxResponseBytes)

	var matches []string
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[1] == assetName {
			matches = append(matches, fields[0])
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("parse SOPS checksums")
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("SOPS checksums must contain exactly one checksum for %q", assetName)
	}
	if !checksumPattern.MatchString(matches[0]) {
		return "", fmt.Errorf("SOPS checksum for %q is not lowercase SHA-256", assetName)
	}
	return matches[0], nil
}

func (c *GitHubClient) getJSON(ctx context.Context, endpoint string, destination any) (http.Header, error) {
	response, err := c.get(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes))
	if err := decoder.Decode(destination); err != nil {
		return nil, fmt.Errorf("decode JSON response")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("decode JSON response")
	}
	return response.Header, nil
}

func (c *GitHubClient) getBytes(ctx context.Context, endpoint string) ([]byte, error) {
	response, err := c.get(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	contents, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read response")
	}
	if len(contents) > maxResponseBytes {
		return nil, fmt.Errorf("response exceeds size limit")
	}
	return contents, nil
}

func (c *GitHubClient) get(ctx context.Context, endpoint string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request")
	}
	request.Header.Set("Accept", gitHubAccept)
	request.Header.Set("X-GitHub-Api-Version", gitHubAPIVersion)
	if c.token != "" && sameOrigin(endpoint, c.baseURL) {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("send request")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		response.Body.Close()
		return nil, fmt.Errorf("unexpected status %d", response.StatusCode)
	}
	return response, nil
}

func sameOrigin(left, right string) bool {
	leftURL, leftErr := url.Parse(left)
	rightURL, rightErr := url.Parse(right)
	return leftErr == nil && rightErr == nil &&
		leftURL.Scheme == rightURL.Scheme && leftURL.Host == rightURL.Host
}

func nextLink(header string) string {
	for _, link := range strings.Split(header, ",") {
		sections := strings.Split(link, ";")
		if len(sections) < 2 || strings.TrimSpace(sections[1]) != `rel="next"` {
			continue
		}
		return strings.Trim(strings.TrimSpace(sections[0]), "<>")
	}
	return ""
}
