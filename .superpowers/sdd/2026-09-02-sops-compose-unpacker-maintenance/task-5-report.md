# Task 5 Report: Verify SOPS and Build the Runtime Image

## Status

Implemented and committed as `cb495ccce39b447c7f73c131a1f9565d0202fd69`
(`build: create verified SOPS runtime image`).

The committed runtime image was built and smoke-tested as
`ghcr.io/jbruns/compose-unpacker:test` for Linux amd64.

## Environment

Every host-side Go operation used `golang:1.26.6` with:

- `--platform linux/amd64`;
- host UID/GID;
- explicit `/usr/local/go/bin/go` and `/usr/local/go/bin/gofmt` paths;
- project-local ignored `.tmp` Go, module, home, and temporary caches;
- Git `safe.directory`;
- no host Go dependency.

The existing linked worktree was
`/Users/jbruns/src/compose-unpacker.worktrees/sops-decryption-portainer-update`
on `agents/sops-decryption-portainer-update`.

## RED/GREEN Evidence

### Verified downloader RED

The downloader tests were written first for successful atomic replacement,
HTTP status failure, network failure, checksum mismatch, executable mode, and
temporary cleanup.

```console
$ docker run --rm --platform linux/amd64 --user "$(id -u):$(id -g)" \
    -e HOME=/repo/.tmp/home -e TMPDIR=/repo/.tmp/test-temp \
    -e GOCACHE=/repo/.tmp/go-build -e GOMODCACHE=/repo/.tmp/go-mod \
    -e GOPATH=/repo/.tmp/go-path \
    -e GIT_CONFIG_COUNT=1 -e GIT_CONFIG_KEY_0=safe.directory \
    -e GIT_CONFIG_VALUE_0='*' -v "$PWD:/repo" -w /repo \
    golang:1.26.6 /usr/local/go/bin/go test ./internal/fetch -count=1
# github.com/jbruns/compose-unpacker-sops/internal/fetch
internal/fetch/fetch_test.go:25:9: undefined: Download
internal/fetch/fetch_test.go:48:9: undefined: Download
internal/fetch/fetch_test.go:74:9: undefined: Download
internal/fetch/fetch_test.go:97:9: undefined: Download
FAIL
```

### Verified downloader GREEN

```console
$ docker run ... golang:1.26.6 /bin/sh -lc \
    '/usr/local/go/bin/gofmt -w cmd/fetch-sops internal/fetch &&
     /usr/local/go/bin/go test ./internal/fetch -count=1 &&
     /usr/local/go/bin/go vet ./cmd/fetch-sops ./internal/fetch'
ok  	github.com/jbruns/compose-unpacker-sops/internal/fetch	0.115s
```

The final race run also passed:

```console
$ docker run ... golang:1.26.6 /usr/local/go/bin/go test \
    -race ./internal/fetch -count=1
ok  	github.com/jbruns/compose-unpacker-sops/internal/fetch	1.155s
```

### Integration failure-path RED sensitivity

The integration failure tests were added before any Task 5 production changes
to decryption. Tasks 3-4 already provided the reviewed fail-closed
implementation, so no new decryption implementation was required. Running the
failure subtests against `/bin/true` proved they reject a runner that falsely
accepts an invalid key or malformed input:

```console
$ docker run --rm --platform linux/amd64 --user "$(id -u):$(id -g)" \
    -e SOPS_BINARY=/bin/true \
    -e HOME=/repo/.tmp/home -e TMPDIR=/repo/.tmp/test-temp \
    -e GOCACHE=/repo/.tmp/go-build -e GOMODCACHE=/repo/.tmp/go-mod \
    -e GOPATH=/repo/.tmp/go-path -v "$PWD:/repo" \
    -w /repo/.work/upstream/compose-unpacker golang:1.26.6 \
    /usr/local/go/bin/go test -tags=integration ./sopsdecrypt \
    -run 'TestDecryptWithRealSOPSAge/(rejects_invalid_key|rejects_malformed_input)$' \
    -count=1
--- FAIL: TestDecryptWithRealSOPSAge
    --- FAIL: TestDecryptWithRealSOPSAge/rejects_invalid_key
        integration_test.go:65: Decrypt() error = <nil>, want SOPS failure
    --- FAIL: TestDecryptWithRealSOPSAge/rejects_malformed_input
        integration_test.go:89: Decrypt() error = <nil>, want SOPS failure
FAIL
```

### Real pinned-SOPS integration GREEN

```console
$ make test-integration
docker run ... /usr/local/go/bin/go run ./cmd/prepare
docker run ... /usr/local/go/bin/go run ./cmd/fetch-sops \
  -output .work/dist/sops
docker run ... /bin/sh -lc \
  'cd /repo/.work/upstream/compose-unpacker &&
   SOPS_BINARY=/repo/.work/dist/sops
   /usr/local/go/bin/go test -tags=integration ./sopsdecrypt -count=1'
ok  	github.com/portainer/compose-unpacker/sopsdecrypt	0.451s
```

The verbose real run showed all three new cases passing:

```text
--- PASS: TestDecryptWithRealSOPSAge
    --- PASS: TestDecryptWithRealSOPSAge/decrypts_age_fixture
    --- PASS: TestDecryptWithRealSOPSAge/rejects_invalid_key
    --- PASS: TestDecryptWithRealSOPSAge/rejects_malformed_input
```

Each failure case asserted that no plaintext destination and no temporary file
remained.

## Pinned SOPS and Fixture Evidence

The manifest-driven downloader produced the exact pinned artifact:

```console
$ docker run ... golang:1.26.6 /bin/sh -lc \
    '/usr/local/go/bin/go run ./cmd/fetch-sops -output .work/tools/sops &&
     .work/tools/sops --version &&
     printf "sha256=" &&
     sha256sum .work/tools/sops | cut -d" " -f1'
sops 3.13.3 (latest)
sha256=e5bec3346a873ae91d871550f3e698c1aad962aff462a080e40f25fde17fef6b
```

`age-keygen@v1.3.1` and the pinned SOPS CLI syntax in the brief both worked
without correction. The committed identity begins with
`# TEST ONLY: this identity protects no real secret.` The expected plaintext is
only `DEMO_VALUE=decrypted-test-value`.

```console
$ docker run --rm --platform linux/amd64 --user "$(id -u):$(id -g)" \
    -e HOME=/repo/.tmp/home -e TMPDIR=/repo/.tmp/test-temp \
    -e GOCACHE=/repo/.tmp/go-build -e GOMODCACHE=/repo/.tmp/go-mod \
    -e GOPATH=/repo/.tmp/go-path -v "$PWD:/repo" -w /repo \
    golang:1.26.6 /bin/sh -lc \
    '/usr/local/go/bin/go run filippo.io/age/cmd/age-keygen@v1.3.1 \
       -o overlay/sopsdecrypt/testdata/age-key.txt;
     printf "# TEST ONLY: this identity protects no real secret.\n" \
       > overlay/sopsdecrypt/testdata/age-key.annotated;
     cat overlay/sopsdecrypt/testdata/age-key.txt \
       >> overlay/sopsdecrypt/testdata/age-key.annotated;
     mv overlay/sopsdecrypt/testdata/age-key.annotated \
       overlay/sopsdecrypt/testdata/age-key.txt;
     AGE_RECIPIENT=$(awk "/public key:/ {print \$4}" \
       overlay/sopsdecrypt/testdata/age-key.txt);
     printf "DEMO_VALUE=decrypted-test-value\n" \
       > overlay/sopsdecrypt/testdata/config.env.expected;
     SOPS_AGE_RECIPIENTS="$AGE_RECIPIENT" .work/tools/sops encrypt \
       --input-type dotenv --output-type dotenv \
       overlay/sopsdecrypt/testdata/config.env.expected \
       > overlay/sopsdecrypt/testdata/config.sops.env'
exit 0; encrypted fixture created
```

## Build and Smoke Evidence

The complete target passed:

```console
$ make validate IMAGE=ghcr.io/jbruns/compose-unpacker:test
ok  	github.com/jbruns/compose-unpacker-sops/internal/fetch
ok  	github.com/jbruns/compose-unpacker-sops/internal/manifest
ok  	github.com/jbruns/compose-unpacker-sops/internal/prepare
ok  	github.com/jbruns/compose-unpacker-sops/overlay/sopsdecrypt
ok  	github.com/portainer/compose-unpacker/commands
ok  	github.com/portainer/compose-unpacker/sopsdecrypt
ok  	github.com/portainer/compose-unpacker/sopsdecrypt  # integration
sops 3.13.3 (latest)
```

The committed source was then rebuilt:

```console
$ make image IMAGE=ghcr.io/jbruns/compose-unpacker:test
#15 exporting manifest sha256:cd2be37e70623d05809a01683ebd08cb5984086f7cfac17aca26491965cac50e done
#15 exporting config sha256:520bbc76243356833daa54bf022574ddc6443060782bddf85a13d1120c8361d0 done
#15 exporting manifest list sha256:928a64e9bf41ce34ddd8025e809f1e99eb770c376425967d3642f299da6d9a89 done
#15 naming to ghcr.io/jbruns/compose-unpacker:test done
```

Final smoke:

```console
$ make test-image IMAGE=ghcr.io/jbruns/compose-unpacker:test
Usage: unpacker <command> [flags]
...
sops 3.13.3 (latest)
```

The smoke script also silently verified the exact entrypoint, amd64
architecture, manifest-derived Portainer/SOPS/overlay/base-digest labels, and
absence of the age fixture names and plaintext content from the final
filesystem.

## Image Digest and Configuration Evidence

```console
$ docker image inspect ghcr.io/jbruns/compose-unpacker:test --format '...'
id=sha256:928a64e9bf41ce34ddd8025e809f1e99eb770c376425967d3642f299da6d9a89
arch=amd64
entrypoint=["/app/compose-unpacker"]
base_digest=sha256:25aea494af4f4f04ce46f9cf4c72e49ed21085cc80e63561cc75292da54bd60a
portainer=2.45.0
sops=v3.13.3
overlay=1
revision=cb495ccce39b447c7f73c131a1f9565d0202fd69
layers=5
```

The runtime's first three filesystem layers exactly equal the pinned base's
three layers, followed by exactly two layers for the replacement
`/app/compose-unpacker` and added `/app/sops`:

```text
base_layer_prefix=true base_layers=3 added_layers=2
```

## Final Verification

```console
$ docker run ... golang:1.26.6 /bin/sh -lc \
    '/usr/local/go/bin/gofmt -w cmd/fetch-sops internal/fetch overlay/sopsdecrypt &&
     /usr/local/go/bin/go test -race ./internal/fetch -count=1 &&
     /usr/local/go/bin/go vet ./cmd/... ./internal/... ./overlay/sopsdecrypt'
ok  	github.com/jbruns/compose-unpacker-sops/internal/fetch	1.155s

$ bash -n scripts/test-image.sh
$ git diff --check
```

All commands exited zero.

## Changed Files

- `.dockerignore`
- `Dockerfile`
- `Makefile`
- `cmd/fetch-sops/main.go`
- `internal/fetch/fetch.go`
- `internal/fetch/fetch_test.go`
- `overlay/sopsdecrypt/integration_test.go`
- `overlay/sopsdecrypt/testdata/age-key.txt`
- `overlay/sopsdecrypt/testdata/config.env.expected`
- `overlay/sopsdecrypt/testdata/config.sops.env`
- `scripts/test-image.sh`

No plan, specification, version manifest, or ledger was changed.

## Self-review

- `fetch.Download` requires HTTP 200, streams to a same-directory temporary
  file and SHA-256 hasher, uses constant-time lowercase checksum comparison,
  applies mode `0755`, syncs, closes, and atomically renames.
- Every downloader error includes the URL host and destination; HTTP response
  bodies are never included.
- Downloader tests preserve old destinations on failure and verify no temporary
  file remains after both success and failure.
- The age identity and expected plaintext are explicitly non-sensitive test
  data. The multi-stage final image copies only the built executable and pinned
  SOPS binary.
- `.dockerignore` is the required root-context allowlist.
- `BASE_IMAGE` is global before both `FROM` instructions. The base digest label
  receives only the digest, not the full base reference.
- The final stage performs no removal and has exactly two added filesystem
  layers over the pinned Portainer base.
- Every manifest value used by Make and smoke validation is obtained through
  `cmd/manifest-value`; no shell JSON parsing was introduced.
- Make and smoke Go operations use the required pinned Linux amd64 container,
  host identity, explicit paths, ignored caches, and Git safe-directory config.
- `clean` removes only `.work` and `coverage.out`.
- Script mode is executable, shell syntax is valid, and the final entrypoint is
  exactly `["/app/compose-unpacker"]`.

## Concerns

- Non-blocking: one post-validation rebuild encountered transient
  `proxy.golang.org` `unexpected EOF` errors. Direct proxy probes immediately
  succeeded, an unchanged retry built cleanly, and the final committed build
  and smoke test passed. Runtime construction therefore still depends on Go
  module network availability during the builder stage.
- Existing repository structure means root `go test ./...` tries to compile
  `overlay/commands` outside its target compose-unpacker module. `make test`
  deliberately tests root-compatible packages and then the complete prepared
  upstream module, matching Tasks 3-4's established validation approach.
