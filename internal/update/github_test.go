package update

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	serverTagSHA    = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	annotatedTagSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	commitTagSHA    = "cccccccccccccccccccccccccccccccccccccccc"
	checksumSHA     = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
)

func TestGitHubClientListsPortainerReleasesWithRequiredHeaders(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/portainer/portainer/releases" {
			http.NotFound(w, r)
			return
		}
		assertGitHubHeaders(t, r, true)
		fmt.Fprint(w, `[
			{"tag_name":"2.39.8","name":"Release 2.39.8 LTS","draft":false,"prerelease":false},
			{"tag_name":"2.45.0","name":"Release 2.45.0 LTS","draft":false,"prerelease":false}
		]`)
	}))
	defer server.Close()

	client := NewGitHubClient(server.Client(), server.URL, "test-token", nil)
	got, err := client.PortainerReleases(context.Background())
	if err != nil {
		t.Fatalf("PortainerReleases() error = %v", err)
	}
	if len(got) != 2 || got[0].TagName != "2.39.8" || got[1].Name != "Release 2.45.0 LTS" {
		t.Fatalf("PortainerReleases() = %#v", got)
	}
}

func TestGitHubClientFollowsReleasePagination(t *testing.T) {
	t.Parallel()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "":
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/portainer/portainer/releases?page=2>; rel="next"`, server.URL))
			fmt.Fprint(w, `[{"tag_name":"2.39.8","name":"Release 2.39.8 LTS","draft":false,"prerelease":false}]`)
		case "2":
			fmt.Fprint(w, `[{"tag_name":"2.45.0","name":"Release 2.45.0 LTS","draft":false,"prerelease":false}]`)
		default:
			http.Error(w, "unexpected page", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client := NewGitHubClient(server.Client(), server.URL, "", nil)
	got, err := client.PortainerReleases(context.Background())
	if err != nil {
		t.Fatalf("PortainerReleases() error = %v", err)
	}
	if len(got) != 2 || got[1].TagName != "2.45.0" {
		t.Fatalf("PortainerReleases() = %#v", got)
	}
}

func TestGitHubClientResolvesLightweightTagToCommit(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/portainer/portainer/git/ref/tags/2.45.0" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"object":{"type":"commit","sha":%q}}`, serverTagSHA)
	}))
	defer server.Close()

	client := NewGitHubClient(server.Client(), server.URL, "", nil)
	got, err := client.TagCommit(context.Background(), "portainer", "2.45.0")
	if err != nil {
		t.Fatalf("TagCommit() error = %v", err)
	}
	if got != serverTagSHA {
		t.Fatalf("TagCommit() = %q, want %q", got, serverTagSHA)
	}
}

func TestGitHubClientDereferencesAnnotatedTagToCommit(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/portainer/compose-unpacker/git/ref/tags/2.45.0":
			fmt.Fprintf(w, `{"object":{"type":"tag","sha":%q}}`, annotatedTagSHA)
		case "/repos/portainer/compose-unpacker/git/tags/" + annotatedTagSHA:
			fmt.Fprintf(w, `{"object":{"type":"commit","sha":%q}}`, commitTagSHA)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewGitHubClient(server.Client(), server.URL, "", nil)
	got, err := client.TagCommit(context.Background(), "compose-unpacker", "2.45.0")
	if err != nil {
		t.Fatalf("TagCommit() error = %v", err)
	}
	if got != commitTagSHA {
		t.Fatalf("TagCommit() = %q, want %q", got, commitTagSHA)
	}
}

func TestGitHubClientRejectsTagThatDoesNotResolveToCommit(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"object":{"type":"tree","sha":%q}}`, serverTagSHA)
	}))
	defer server.Close()

	client := NewGitHubClient(server.Client(), server.URL, "", nil)
	_, err := client.TagCommit(context.Background(), "portainer", "2.45.0")
	if err == nil {
		t.Fatal("TagCommit() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "does not resolve to a commit") {
		t.Fatalf("TagCommit() error = %q", err)
	}
}

func TestGitHubClientResolvesExactSOPSAssetAndChecksum(t *testing.T) {
	t.Parallel()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/getsops/sops/releases/latest":
			assertGitHubHeaders(t, r, true)
			fmt.Fprintf(w, `{
				"tag_name":"v3.13.3",
				"draft":false,
				"prerelease":false,
				"assets":[
					{"name":"sops-v3.13.3.linux.amd64.sig","browser_download_url":%q},
					{"name":"sops-v3.13.3.linux.amd64","browser_download_url":%q},
					{"name":"sops-v3.13.3.checksums.txt","browser_download_url":%q}
				]
			}`, server.URL+"/download/signature", server.URL+"/download/custom-binary", server.URL+"/sops-v3.13.3.checksums.txt")
		case "/sops-v3.13.3.checksums.txt":
			assertGitHubHeaders(t, r, true)
			fmt.Fprintf(w, "%s  sops-v3.13.3.linux.arm64\n%s  sops-v3.13.3.linux.amd64\n", strings.Repeat("e", 64), checksumSHA)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewGitHubClient(server.Client(), server.URL, "test-token", nil)
	got, err := client.LatestSOPS(context.Background())
	if err != nil {
		t.Fatalf("LatestSOPS() error = %v", err)
	}
	want := SOPSRelease{
		Version: "v3.13.3",
		Asset:   "sops-v3.13.3.linux.amd64",
		URL:     server.URL + "/download/custom-binary",
		SHA256:  checksumSHA,
	}
	if got != want {
		t.Fatalf("LatestSOPS() = %#v, want %#v", got, want)
	}
}

func TestGitHubClientRejectsAmbiguousOrMissingSOPSInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		assets        string
		checksums     string
		want          string
		serveChecksum bool
	}{
		{
			name: "missing exact binary asset",
			assets: `[
				{"name":"sops-v3.13.3.linux.amd64.sig","browser_download_url":"ASSET_URL"},
				{"name":"sops-v3.13.3.checksums.txt","browser_download_url":"CHECKSUM_URL"}
			]`,
			want: "exactly one asset named",
		},
		{
			name: "duplicate binary assets",
			assets: `[
				{"name":"sops-v3.13.3.linux.amd64","browser_download_url":"ASSET_URL"},
				{"name":"sops-v3.13.3.linux.amd64","browser_download_url":"ASSET_URL_2"},
				{"name":"sops-v3.13.3.checksums.txt","browser_download_url":"CHECKSUM_URL"}
			]`,
			want: "exactly one asset named",
		},
		{
			name: "duplicate checksum assets",
			assets: `[
				{"name":"sops-v3.13.3.linux.amd64","browser_download_url":"ASSET_URL"},
				{"name":"sops-v3.13.3.checksums.txt","browser_download_url":"CHECKSUM_URL"},
				{"name":"sops-v3.13.3.checksums.txt","browser_download_url":"CHECKSUM_URL_2"}
			]`,
			want: "exactly one asset named",
		},
		{
			name: "missing exact checksum field",
			assets: `[
				{"name":"sops-v3.13.3.linux.amd64","browser_download_url":"ASSET_URL"},
				{"name":"sops-v3.13.3.checksums.txt","browser_download_url":"CHECKSUM_URL"}
			]`,
			checksums:     strings.Repeat("a", 64) + "  sops-v3.13.3.linux.amd64.sig\n",
			want:          "exactly one checksum",
			serveChecksum: true,
		},
		{
			name: "duplicate exact checksum fields",
			assets: `[
				{"name":"sops-v3.13.3.linux.amd64","browser_download_url":"ASSET_URL"},
				{"name":"sops-v3.13.3.checksums.txt","browser_download_url":"CHECKSUM_URL"}
			]`,
			checksums:     checksumSHA + "  sops-v3.13.3.linux.amd64\n" + checksumSHA + "  sops-v3.13.3.linux.amd64\n",
			want:          "exactly one checksum",
			serveChecksum: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/getsops/sops/releases/latest":
					assets := strings.NewReplacer(
						"ASSET_URL_2", server.URL+"/asset-2",
						"ASSET_URL", server.URL+"/asset",
						"CHECKSUM_URL_2", server.URL+"/checksum-2",
						"CHECKSUM_URL", server.URL+"/checksum",
					).Replace(tt.assets)
					fmt.Fprintf(w, `{"tag_name":"v3.13.3","draft":false,"prerelease":false,"assets":%s}`, assets)
				case "/checksum":
					if !tt.serveChecksum {
						http.NotFound(w, r)
						return
					}
					fmt.Fprint(w, tt.checksums)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			client := NewGitHubClient(server.Client(), server.URL, "", nil)
			_, err := client.LatestSOPS(context.Background())
			if err == nil {
				t.Fatal("LatestSOPS() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LatestSOPS() error = %q, want substring %q", err, tt.want)
			}
		})
	}
}

func TestGitHubClientErrorsDoNotLeakBodyOrToken(t *testing.T) {
	t.Parallel()

	const responseSecret = "response-body-secret"
	const token = "bearer-token-secret"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, responseSecret, http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewGitHubClient(server.Client(), server.URL, token, nil)
	_, err := client.PortainerReleases(context.Background())
	if err == nil {
		t.Fatal("PortainerReleases() error = nil, want error")
	}
	if strings.Contains(err.Error(), responseSecret) || strings.Contains(err.Error(), token) {
		t.Fatalf("PortainerReleases() leaked sensitive response data: %q", err)
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("PortainerReleases() error = %q, want status", err)
	}
}

func assertGitHubHeaders(t *testing.T, r *http.Request, wantAuth bool) {
	t.Helper()

	if r.Header.Get("Accept") != "application/vnd.github+json" {
		t.Errorf("Accept header is missing")
	}
	if r.Header.Get("X-GitHub-Api-Version") != "2022-11-28" {
		t.Errorf("X-GitHub-Api-Version header is missing")
	}
	if got := r.Header.Get("Authorization"); (got != "") != wantAuth {
		t.Errorf("Authorization header presence = %t, want %t", got != "", wantAuth)
	}
}
