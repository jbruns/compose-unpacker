package fetch

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

func Download(
	ctx context.Context,
	client *http.Client,
	sourceURL, destination, expectedSHA256 string,
) error {
	host := sourceURL
	if parsed, err := url.Parse(sourceURL); err == nil && parsed.Host != "" {
		host = parsed.Host
	}
	fail := func(format string, args ...any) error {
		return fmt.Errorf("download from %s to %s: %s", host, destination, fmt.Sprintf(format, args...))
	}
	if client == nil {
		return fail("HTTP client must not be nil")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return fail("create request: %v", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return fail("request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fail("unexpected HTTP status %d", response.StatusCode)
	}

	temporary, err := os.CreateTemp(
		filepath.Dir(destination),
		"."+filepath.Base(destination)+".tmp-*",
	)
	if err != nil {
		return fail("create temporary file: %v", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()

	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(temporary, hasher), response.Body); err != nil {
		return fail("write temporary file: %v", err)
	}
	actualSHA256 := hex.EncodeToString(hasher.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(actualSHA256), []byte(expectedSHA256)) != 1 {
		return fail("checksum mismatch: got %s, want %s", actualSHA256, expectedSHA256)
	}
	if err := temporary.Chmod(0o755); err != nil {
		return fail("set temporary file mode: %v", err)
	}
	if err := temporary.Sync(); err != nil {
		return fail("sync temporary file: %v", err)
	}
	if err := temporary.Close(); err != nil {
		return fail("close temporary file: %v", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return fail("promote temporary file: %v", err)
	}

	return nil
}
