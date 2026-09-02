# Task 6 Report: Resolve Portainer LTS and SOPS Updates

## Status

Complete. The updater selects the highest numeric three-part Portainer release
whose name contains a whitespace-delimited `LTS` token, resolves both tag
commits and the single Linux/amd64 image child, verifies the exact SOPS binary
and checksum, and supports deterministic check/write CLI modes.

The live check on 2026-09-02 found no update beyond Portainer `2.45.0` and SOPS
`v3.13.3`, so `versions.json` remained byte-for-byte unchanged.

## Commits

- `12a71f6` — `feat: resolve Portainer LTS and SOPS updates`
- `19e5472` — `fix: compare unbounded release version components`

## TDD Evidence

All Go commands used this Linux/amd64 Go 1.26.6 container configuration:

```sh
docker run --rm --platform linux/amd64 \
  --user "$(id -u):$(id -g)" \
  -e PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin \
  -e HOME=/repo/.tmp/home \
  -e TMPDIR=/repo/.tmp/test-temp \
  -e GOCACHE=/repo/.tmp/go-build \
  -e GOMODCACHE=/repo/.tmp/go-mod \
  -e GOPATH=/repo/.tmp/go-path \
  -e GIT_CONFIG_COUNT=1 \
  -e GIT_CONFIG_KEY_0=safe.directory \
  -e GIT_CONFIG_VALUE_0='*' \
  -v "$PWD:/repo" -w /repo \
  golang:1.26.6
```

### Resolver

RED:

```sh
/usr/local/go/bin/go test ./internal/update -count=1
```

Result: exit `1`; `resolver_test.go` failed to compile with the expected
`undefined: Release` and `undefined: Resolve` errors.

GREEN:

```sh
/usr/local/go/bin/go test ./internal/update -count=1
```

Result:

```text
ok github.com/jbruns/compose-unpacker-sops/internal/update 0.041s
```

Covered numeric ordering, LTS token matching, malformed/draft/prerelease/STS
filtering, missing LTS and source errors, revision reset/increment behavior,
immutable inputs, summaries, and exact no-change results.

### GitHub client

RED:

```sh
/usr/local/go/bin/go test ./internal/update -count=1
```

Result: exit `1`; all GitHub fixtures failed to compile with the expected
`undefined: NewGitHubClient`.

GREEN:

```sh
/usr/local/go/bin/go test ./internal/update -count=1
```

Result:

```text
ok github.com/jbruns/compose-unpacker-sops/internal/update 0.260s
```

The `httptest.Server` fixtures cover required API headers and optional auth,
release pagination, lightweight and annotated tags, commit-only refs, exact
SOPS asset and checksum selection, duplicate rejection, and non-leaking HTTP
errors.

### Image resolver

RED:

```sh
/usr/local/go/bin/go test ./internal/update -count=1
```

Result: exit `1`; image tests failed to compile with the expected
`undefined: LinuxAMD64Digest`.

GREEN:

```sh
/usr/local/go/bin/go test ./internal/update -count=1
```

Result:

```text
ok github.com/jbruns/compose-unpacker-sops/internal/update 0.239s
```

Covered successful OCI index parsing, absent and duplicate Linux/amd64
children, malformed JSON/index/digests, and inspection failures. The production
inspector executes:

```text
docker buildx imagetools inspect --format {{json .Manifest}} <image>:<tag>
```

### CLI

RED:

```sh
/usr/local/go/bin/go test ./cmd/update-versions -count=1
```

Result: exit `1`; CLI tests failed to compile with the expected `undefined:
run` and `undefined: runWithRename`.

GREEN:

```sh
/usr/local/go/bin/go test ./cmd/update-versions -count=1
```

Result:

```text
ok github.com/jbruns/compose-unpacker-sops/cmd/update-versions 0.056s
```

Covered mutually exclusive modes, documented exit codes, no-change byte
preservation, check-mode non-writing, deterministic formatted output, atomic
same-directory replacement, and cleanup after an injected rename failure.

### Self-review regression

Self-review found that `strconv.Atoi` imposed a machine-size limit not present
in the numeric three-part version requirement.

RED:

```sh
/usr/local/go/bin/go test ./internal/update \
  -run TestResolveComparesNumericComponentsWithoutMachineIntegerLimit -count=1
```

Result: exit `1`:

```text
Portainer.Version = "999.0.0", want "184467440737095516160.0.0"
```

GREEN:

```sh
/usr/local/go/bin/gofmt -w internal/update/resolver.go \
  internal/update/resolver_test.go
/usr/local/go/bin/go test ./internal/update -count=1
```

Result:

```text
ok github.com/jbruns/compose-unpacker-sops/internal/update 0.244s
```

The resolver now normalizes and compares decimal components by digit length and
lexical value without adding a dependency.

## Final Validation

Exact final container command body:

```sh
test -z "$(/usr/local/go/bin/gofmt -l cmd/update-versions internal/update)" &&
/usr/local/go/bin/go test ./cmd/... ./internal/... ./overlay/sopsdecrypt -count=1 &&
/usr/local/go/bin/go test -race ./cmd/update-versions ./internal/update -count=1 &&
/usr/local/go/bin/go vet ./cmd/update-versions ./internal/update
```

Result: exit `0`. All command and internal package tests passed, including:

```text
ok github.com/jbruns/compose-unpacker-sops/cmd/update-versions 0.081s
ok github.com/jbruns/compose-unpacker-sops/internal/update 0.283s
ok github.com/jbruns/compose-unpacker-sops/cmd/update-versions 1.098s [race]
ok github.com/jbruns/compose-unpacker-sops/internal/update 1.347s [race]
```

`go vet` was silent and exited `0`. `git diff --check` was also silent and
exited `0`.

## Live API and Image Result

Because the stock Go image does not include Docker, the live `go run` used a
temporary local image based on `golang:1.26.6` with only the Docker CLI and
Buildx plugin copied from `docker:cli`. The image was removed after the check.
The runtime retained Linux/amd64, host UID/GID, explicit Go paths, repository
caches, `safe.directory`, and the Docker socket:

```sh
docker buildx build --platform linux/amd64 --load \
  --tag compose-unpacker-update-tools:task6 - <<'EOF'
FROM docker:cli AS docker
FROM golang:1.26.6
COPY --from=docker /usr/local/bin/docker /usr/local/bin/docker
COPY --from=docker /usr/local/libexec/docker/cli-plugins/docker-buildx /usr/local/libexec/docker/cli-plugins/docker-buildx
EOF

docker run --rm --platform linux/amd64 \
  --user "$(id -u):$(id -g)" \
  -e PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin \
  -e HOME=/repo/.tmp/home \
  -e TMPDIR=/repo/.tmp/test-temp \
  -e GOCACHE=/repo/.tmp/go-build \
  -e GOMODCACHE=/repo/.tmp/go-mod \
  -e GOPATH=/repo/.tmp/go-path \
  -e GITHUB_TOKEN \
  -e GIT_CONFIG_COUNT=1 \
  -e GIT_CONFIG_KEY_0=safe.directory \
  -e GIT_CONFIG_VALUE_0='*' \
  -v "$PWD:/repo" \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -w /repo compose-unpacker-update-tools:task6 \
  /usr/local/go/bin/go run ./cmd/update-versions \
    -manifest versions.json -check
```

Result: exit `0`:

```json
{"changed":false,"portainerBefore":"2.45.0","portainerAfter":"2.45.0","sopsBefore":"v3.13.3","sopsAfter":"v3.13.3","overlayRevision":1}
```

No token value or response body was logged.

## Changed Files

- `README.md`
- `cmd/update-versions/main.go`
- `cmd/update-versions/main_test.go`
- `internal/update/github.go`
- `internal/update/github_test.go`
- `internal/update/image.go`
- `internal/update/image_test.go`
- `internal/update/resolver.go`
- `internal/update/resolver_test.go`
- `.superpowers/sdd/2026-09-02-sops-compose-unpacker-maintenance/task-6-report.md`

`versions.json` was checked live but did not change. `go.mod` and `go.sum` did
not change; the implementation uses only the standard library.

## Self-review

- Re-read every binding decision and Task 6 checklist item against the
  implementation and tests.
- Confirmed release selection is numeric and independent of publication order.
- Confirmed tag resolution accepts commits only and dereferences annotated tags.
- Confirmed exact asset/checksum matching rejects missing and duplicate entries.
- Confirmed only one Linux/amd64 OCI child digest is accepted.
- Confirmed no-change write mode performs no filesystem replacement.
- Confirmed the temporary file is same-directory, synced, closed, renamed, and
  removed after failures.
- Confirmed HTTP errors exclude response bodies and request errors do not expose
  tokens; bearer auth is restricted to the configured API origin.
- Confirmed subprocess execution does not invoke a shell.
- Confirmed temporary live-check artifacts were removed.

## Concerns

None.
