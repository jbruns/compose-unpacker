# SOPS Compose Unpacker Maintenance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the stale 2.26 source fork with a reproducible Linux amd64 overlay for Portainer 2.45.0 that decrypts `*.sops.*` files and automatically proposes future LTS and SOPS updates.

**Architecture:** The maintained branch owns a version manifest, focused Go maintenance tools, overlay source, a minimal upstream integration patch, container build files, tests, and workflows. Preparation fetches immutable Portainer sources, applies the overlay, and builds a replacement binary; the final image inherits the matching official compose-unpacker image by digest and adds only that binary and SOPS.

**Tech Stack:** Go 1.26.6, SOPS 3.13.3, age, Git, Docker Buildx, Bash, GitHub Actions, GHCR

**Spec:** `docs/superpowers/specs/2026-09-02-sops-compose-unpacker-maintenance-design.md`

## Global Constraints

- The initial Portainer LTS is exactly `2.45.0`.
- The initial compose-unpacker commit is `23c8e42176c521cb6745b3ea95233d3a68bbe031`.
- The initial Portainer commit is `d79ba726cd54395a54cca5e9180609ce52fa7a4f`.
- The vanilla Linux amd64 image is `docker.io/portainer/compose-unpacker@sha256:25aea494af4f4f04ce46f9cf4c72e49ed21085cc80e63561cc75292da54bd60a`.
- The initial SOPS asset is `sops-v3.13.3.linux.amd64` with SHA-256 `e5bec3346a873ae91d871550f3e698c1aad962aff462a080e40f25fde17fef6b`.
- Only Linux amd64 is supported.
- Portainer and SOPS inputs must be pinned to commits, digests, and checksums; no build may silently fall back to a mutable reference.
- Runtime discovery is recursive below each Compose file directory, matches `*.sops.*`, skips `.git`, does not follow symlinks, and fails closed.
- Runtime logs must not contain deployment environment values, age keys, command environments, or plaintext.
- GHCR publication happens only after merge to `main` and uses `GITHUB_TOKEN`.
- Immutable tags use `<portainer-version>-sops.<overlay-revision>` and must never be overwritten.
- Use TDD for every behavior change and commit after every task.

---

## File Structure

The finished maintained tree will have these responsibilities:

```text
.github/
  dependabot.yml                 # GitHub Actions update policy
  workflows/
    ci.yml                       # PR/manual validation
    update.yml                   # weekly LTS/SOPS update PR
    release.yml                  # post-merge GHCR publication and attestation
cmd/
  fetch-sops/main.go             # checksum-verified SOPS download CLI
  manifest-value/main.go         # safe script access to manifest fields
  prepare/main.go                # immutable upstream preparation CLI
  update-versions/main.go        # release resolution and manifest update CLI
internal/
  fetch/fetch.go                 # atomic verified download
  fetch/fetch_test.go
  manifest/manifest.go           # versions.json model and validation
  manifest/manifest_test.go
  prepare/prepare.go             # fetch/copy/patch orchestration
  prepare/prepare_test.go
  update/resolver.go             # release selection and revision rules
  update/resolver_test.go
  update/github.go               # GitHub release/tag/checksum client
  update/github_test.go
  update/image.go                # Docker manifest resolver
  update/image_test.go
overlay/
  commands/
    sops.go                       # shared command-package integration helper
    sops_test.go                  # error wrapping and argument tests
  sopsdecrypt/
    decrypt.go                   # atomic fail-closed decryption orchestration
    decrypt_test.go
    discover.go                  # root and encrypted-file discovery
    discover_test.go
    runner.go                    # context-aware SOPS process execution
    runner_test.go
    integration_test.go          # real SOPS + age test, integration build tag
    testdata/
      age-key.txt                # test-only non-production age identity
      config.sops.env            # encrypted non-sensitive fixture
      config.env.expected        # expected fixture plaintext
patches/
  compose-unpacker.patch         # minimal Compose/Swarm call-site edits
scripts/
  test-image.sh                  # image entrypoint/version/label smoke tests
  validate.sh                    # one local/CI validation entry point
.dockerignore                    # limits the image build context
.gitignore                       # excludes .work and generated plaintext
Dockerfile                       # build patched binary; extend pinned vanilla image
Makefile                         # prepare, test, image, validate targets
README.md                        # user and maintainer workflow
go.mod                           # maintenance-tool module
go.sum                           # maintenance-tool dependencies, if any
versions.json                    # all immutable external inputs and overlay revision
```

The old vendored upstream source files and obsolete build scripts are removed.
The generated `.work/upstream` tree is the only place upstream source exists
during a build.

---

### Task 1: Replace the Fork with a Validated Version Manifest

**Files:**
- Create: `versions.json`
- Create: `cmd/manifest-value/main.go`
- Create: `cmd/manifest-value/main_test.go`
- Create: `internal/manifest/manifest.go`
- Create: `internal/manifest/manifest_test.go`
- Replace: `go.mod`
- Delete: `go.sum`
- Modify: `.gitignore`
- Delete: `.github/workflows/dev.workflow.yaml`
- Delete: `.github/workflows/prod.workflow.yaml`
- Delete: `build/build_and_push.sh`
- Delete: `build/download_docker_binary.sh`
- Delete: `build/download_sops_binary.sh`
- Delete: `build/linux/Dockerfile`
- Delete: `build/windows/Dockerfile`
- Delete: `deploy.go`
- Delete: `log/log.go`
- Delete: `main.go`
- Delete: `non-root.Dockerfile`
- Delete: `removedir.go`
- Delete: `setup.sh`
- Delete: `swarm.go`
- Delete: `types.go`
- Delete: `undeploy.go`

**Interfaces:**
- Produces: `manifest.Load(path string) (Manifest, error)`
- Produces: `Manifest.Validate() error`
- Produces: `Manifest.ImmutableTag() string`
- Produces: `Manifest.VersionTag() string`
- Produces: `Manifest.BaseImage() string`
- Produces: `go run ./cmd/manifest-value <field>`

- [ ] **Step 1: Write manifest validation tests**

Create `internal/manifest/manifest_test.go` with table tests for a valid
manifest, malformed SHA/digest/checksum values, unsupported platforms,
non-positive overlay revisions, mismatched SOPS asset names, and generated
tags:

```go
func TestManifestValidate(t *testing.T) {
	t.Parallel()

	valid := validManifest()

	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if got := valid.ImmutableTag(); got != "2.45.0-sops.1" {
		t.Fatalf("ImmutableTag() = %q", got)
	}
	if got := valid.BaseImage(); got != valid.Portainer.Image+"@"+valid.Portainer.LinuxAMD64Digest {
		t.Fatalf("BaseImage() = %q", got)
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
```

Run `go run ./cmd/prepare` once after creating the helper so it is copied into
the generated command package, then edit the two generated deployment files.

- [ ] **Step 2: Run the manifest tests and verify they fail**

Run:

```bash
go test ./internal/manifest
```

Expected: FAIL because the maintenance module and `manifest` package do not
exist.

- [ ] **Step 3: Replace the root module and implement the manifest**

Replace `go.mod` with:

```go
module github.com/jbruns/compose-unpacker-sops

go 1.26.6
```

Implement strict JSON decoding with `json.Decoder.DisallowUnknownFields()`.
Reject trailing JSON, require exactly `linux/amd64`, validate versions with
anchored regular expressions, and validate every commit/checksum as lowercase
hex of the required length.

Use these public types:

```go
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
```

Create `versions.json` using the exact values from the test fixture. Make
`Load` return contextual errors such as `decode versions.json: ...` and
`validate versions.json: ...`.

- [ ] **Step 4: Implement the strict manifest accessor**

Create `cmd/manifest-value/main.go`. It loads `versions.json` by default,
accepts `-manifest` to override that path, requires one positional field, and
prints exactly one value plus a newline. Support:

```text
go-version
lint-version
base-image
base-digest
portainer-version
sops-version
overlay-revision
immutable-tag
version-tag
```

Unknown fields and extra arguments exit non-zero. Add command-level tests by
extracting `run(args []string, stdout, stderr io.Writer) int`.

- [ ] **Step 5: Remove the copied upstream source and ignore generated state**

Delete the files listed above. Preserve `README.md`, `Makefile`, and the design
documents because later tasks replace or extend them.

Add these entries to `.gitignore`:

```gitignore
.work/
coverage.out
*.tmp
overlay/sopsdecrypt/testdata/config.env
```

- [ ] **Step 6: Run manifest tests and repository checks**

Run:

```bash
gofmt -w cmd/manifest-value internal/manifest
go test ./cmd/manifest-value ./internal/manifest
go vet ./cmd/manifest-value ./internal/manifest
test "$(go run ./cmd/manifest-value portainer-version)" = "2.45.0"
git diff --check
```

Expected: all commands succeed.

- [ ] **Step 7: Commit the overlay skeleton**

```bash
git add -A
git commit -m "refactor: replace source fork with overlay skeleton"
```

---

### Task 2: Prepare Immutable Upstream Source Trees

**Files:**
- Create: `internal/prepare/prepare.go`
- Create: `internal/prepare/prepare_test.go`
- Create: `cmd/prepare/main.go`
- Create: `patches/compose-unpacker.patch`

**Interfaces:**
- Consumes: `manifest.Load(path string) (manifest.Manifest, error)`
- Produces: `prepare.Options{Root, WorkDir string; Manifest manifest.Manifest}`
- Produces: `prepare.Run(ctx context.Context, options Options, runner Runner) error`
- Produces: `prepare.ExecRunner`

- [ ] **Step 1: Write failing orchestration tests**

Define a fake runner that records commands and supplies output. Test that
`Run`:

- removes only the configured work directory;
- fetches `refs/tags/2.45.0` from both upstream repositories;
- verifies each fetched commit against the manifest;
- changes the Portainer module replacement to `../portainer`;
- refuses to overwrite an upstream path while copying `overlay`;
- runs `git apply --check` before `git apply`; and
- reports the failing stage and command stderr.

The success assertion must include this order:

```go
wantCommands := []string{
	"git init",
	"git remote add origin https://github.com/portainer/compose-unpacker.git",
	"git fetch --depth=1 origin refs/tags/2.45.0",
	"git checkout --detach 23c8e42176c521cb6745b3ea95233d3a68bbe031",
	"git init",
	"git remote add origin https://github.com/portainer/portainer.git",
	"git fetch --depth=1 origin refs/tags/2.45.0",
	"git checkout --detach d79ba726cd54395a54cca5e9180609ce52fa7a4f",
	"go mod edit -replace=github.com/portainer/portainer=../portainer",
	"git apply --check " + filepath.Join(root, "patches/compose-unpacker.patch"),
	"git apply " + filepath.Join(root, "patches/compose-unpacker.patch"),
}
```

- [ ] **Step 2: Run preparation tests and verify they fail**

Run:

```bash
go test ./internal/prepare
```

Expected: FAIL because `prepare.Run` is undefined.

- [ ] **Step 3: Implement the runner and preparation pipeline**

Use this runner boundary:

```go
type Runner interface {
	Run(ctx context.Context, dir, name string, args ...string) error
	Output(ctx context.Context, dir, name string, args ...string) ([]byte, error)
}

type Options struct {
	Root     string
	WorkDir string
	Manifest manifest.Manifest
}
```

`ExecRunner` must use `exec.CommandContext`, capture stderr, and wrap failures
with the executable, arguments, and stderr. It must never include process
environment values.

Implement `Run` as focused helpers:

```go
func Run(ctx context.Context, options Options, runner Runner) error
func fetchTaggedTree(ctx context.Context, runner Runner, destination, repository, version, commit string) error
func copyOverlay(source, destination string) ([]string, error)
func applyPatch(ctx context.Context, runner Runner, repository, patch string) error
func verifyChanges(ctx context.Context, runner Runner, repository string, allowed []string) error
```

Resolve `Root` and `WorkDir` with `filepath.Abs`, reject an empty/root work
directory, and remove only the resolved work directory. Store source at:

```text
.work/upstream/compose-unpacker
.work/upstream/portainer
```

If `overlay/` exists, copy `overlay/**` into
`.work/upstream/compose-unpacker/**`; an absent overlay is valid until Task 3.
Start
`patches/compose-unpacker.patch` with the first real security integration:
replace `Strs("env", cmd.Env)` in `commands/compose_deploy.go` with
`Int("envCount", len(cmd.Env))`. Task 4 extends this patch with the SOPS call
sites; it must preserve this redaction.

- [ ] **Step 4: Implement the CLI**

`cmd/prepare/main.go` must accept:

```text
-manifest versions.json
-work-dir .work
```

It loads and validates the manifest, derives the repository root from the
current working directory, invokes `prepare.Run`, writes errors to stderr, and
exits non-zero. It must not catch an error and emit success-shaped output.

- [ ] **Step 5: Run unit and real preparation tests**

Run:

```bash
gofmt -w cmd/prepare internal/prepare
go test ./internal/manifest ./internal/prepare
go run ./cmd/prepare
test "$(git -C .work/upstream/compose-unpacker rev-parse HEAD)" = \
  "23c8e42176c521cb6745b3ea95233d3a68bbe031"
test "$(git -C .work/upstream/portainer rev-parse HEAD)" = \
  "d79ba726cd54395a54cca5e9180609ce52fa7a4f"
git diff --check
```

Expected: all commands succeed and the generated tree is dirty only for the
module replacement and the environment-log redaction patch.

- [ ] **Step 6: Commit source preparation**

```bash
git add cmd/prepare internal/prepare patches/compose-unpacker.patch
git commit -m "build: prepare immutable Portainer sources"
```

---

### Task 3: Implement Deterministic SOPS Discovery and Atomic Decryption

**Files:**
- Create: `overlay/sopsdecrypt/discover.go`
- Create: `overlay/sopsdecrypt/discover_test.go`
- Create: `overlay/sopsdecrypt/decrypt.go`
- Create: `overlay/sopsdecrypt/decrypt_test.go`
- Create: `overlay/sopsdecrypt/runner.go`
- Create: `overlay/sopsdecrypt/runner_test.go`

**Interfaces:**
- Produces: `sopsdecrypt.Decrypt(ctx context.Context, composeFiles, env []string, binary string) error`
- Produces: `sopsdecrypt.DefaultBinaryPath = "/app/sops"`
- Internal: `runner.Decrypt(ctx context.Context, source string, destination io.Writer, env []string) error`

- [ ] **Step 1: Write failing discovery tests**

Cover root reduction, path boundaries, duplicate inputs, `.git` exclusion,
symlinks, deterministic sorting, naming, duplicate outputs, and walk errors.

Use explicit cases:

```go
func TestRootDirectoriesPreservePathBoundaries(t *testing.T) {
	t.Parallel()
	got, err := rootDirectories([]string{
		"/stacks/a/compose.yaml",
		"/stacks/a/nested/compose.yaml",
		"/stacks/ab/compose.yaml",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/stacks/a", "/stacks/ab"}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("roots = %v, want %v", got, want)
	}
}

func TestOutputPathReplacesFirstMarker(t *testing.T) {
	t.Parallel()
	got, err := outputPath("/repo/app.sops.env")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/repo/app.env" {
		t.Fatalf("outputPath() = %q", got)
	}
}
```

Use `reflect.DeepEqual` because the overlay must not add test dependencies to
the upstream module.

- [ ] **Step 2: Run discovery tests and verify they fail**

Copy the new files into a prepared tree, then run:

```bash
go run ./cmd/prepare
(cd .work/upstream/compose-unpacker && go test ./sopsdecrypt)
```

Expected: FAIL because discovery functions are undefined.

- [ ] **Step 3: Implement path-aware discovery**

Implement:

```go
type encryptedFile struct {
	Source      string
	Destination string
}

type walkDirFunc func(root string, fn fs.WalkDirFunc) error

func rootDirectories(composeFiles []string) ([]string, error)
func discover(composeFiles []string, walk walkDirFunc) ([]encryptedFile, error)
func outputPath(source string) (string, error)
```

Use `filepath.Rel` to test containment. A root contains a candidate only when
the relative path is `"."` or does not begin with `..` plus a path separator.
Use `filepath.WalkDir`, return `filepath.SkipDir` for `.git`, require
`entry.Type().IsRegular()`, and propagate callback and walk errors. Sort roots
and source paths. Build the complete destination map before decrypting and
return an error that names both colliding sources.

- [ ] **Step 4: Write failing runner and atomic-output tests**

Use a fake runner that writes known bytes or returns a sentinel error. Cover:

- file mode `0600`;
- replacement of an existing regular file;
- replacement of a destination symlink without modifying its target;
- removal of temporary files on process, write, sync, close, and rename
  failures through injected file operations;
- no destination creation after runner failure;
- context cancellation; and
- error strings that exclude a supplied `SOPS_AGE_KEY=secret-test-key`.

The core success test:

```go
func TestDecryptFilesAtomicallyReplacesDestination(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	source := filepath.Join(root, "app.sops.env")
	destination := filepath.Join(root, "app.env")
	writeFile(t, source, []byte("encrypted"), 0o600)
	writeFile(t, destination, []byte("old"), 0o600)

	err := decryptFiles(
		context.Background(),
		[]encryptedFile{{Source: source, Destination: destination}},
		[]string{"SOPS_AGE_KEY=secret-test-key"},
		fakeRunner{plaintext: []byte("VALUE=decrypted\n")},
		defaultFileOps(),
	)
	if err != nil {
		t.Fatal(err)
	}
	assertFile(t, destination, "VALUE=decrypted\n", 0o600)
	assertNoTemporaryFiles(t, root)
}
```

- [ ] **Step 5: Implement SOPS execution and atomic promotion**

Use:

```go
const DefaultBinaryPath = "/app/sops"

type runner interface {
	Decrypt(ctx context.Context, source string, destination io.Writer, env []string) error
}

type binaryRunner struct {
	binary string
}

type fileOps struct {
	CreateTemp func(directory, pattern string) (*os.File, error)
	Chmod      func(file *os.File, mode fs.FileMode) error
	Sync       func(file *os.File) error
	Close      func(file *os.File) error
	Rename     func(oldPath, newPath string) error
	Remove     func(path string) error
}

func Decrypt(ctx context.Context, composeFiles, env []string, binary string) error
func decryptFiles(ctx context.Context, files []encryptedFile, env []string, runner runner, ops fileOps) error
func defaultFileOps() fileOps
```

`binaryRunner.Decrypt` must execute:

```text
<binary> decrypt <absolute-source>
```

Set `cmd.Env` to a copy of the supplied deployment environment, stream stdout
to the temporary file, capture only stderr, and redact stderr from the returned
error because provider tooling can echo sensitive values. Return
`decrypt <source>: sops exited with status N`.

Create each temporary file in the destination directory with `os.CreateTemp`,
force mode `0600`, call `Sync` and `Close`, then use `os.Rename`. Defer cleanup
immediately after creation and cancel that cleanup only after a successful
rename. Wrap each error with the source path and stage.

- [ ] **Step 6: Run all overlay unit tests**

Run:

```bash
gofmt -w overlay/sopsdecrypt
go run ./cmd/prepare
(cd .work/upstream/compose-unpacker && go test -race ./sopsdecrypt)
(cd .work/upstream/compose-unpacker && go vet ./sopsdecrypt)
git diff --check
```

Expected: all commands succeed.

- [ ] **Step 7: Commit runtime decryption**

```bash
git add overlay/sopsdecrypt
git commit -m "feat: add fail-closed SOPS decryption"
```

---

### Task 4: Integrate Decryption with Compose and Swarm

**Files:**
- Create: `overlay/commands/sops.go`
- Create: `overlay/commands/sops_test.go`
- Replace: `patches/compose-unpacker.patch`
- Test generated: `.work/upstream/compose-unpacker/commands/compose_deploy.go`
- Test generated: `.work/upstream/compose-unpacker/commands/swarm_deploy.go`

**Interfaces:**
- Consumes: `sopsdecrypt.Decrypt(ctx context.Context, composeFiles, env []string, binary string) error`
- Consumes: `sopsdecrypt.DefaultBinaryPath`
- Produces: `commands.decryptSOPSFiles(ctx context.Context, composeFiles, env []string) error`
- Produces: both deployment paths return before deployer invocation when the helper fails

- [ ] **Step 1: Write failing command-helper tests**

Create `overlay/commands/sops_test.go`. The persistent helper will expose this
package variable for tests:

```go
var decryptSOPS = sopsdecrypt.Decrypt
```

Tests replace `decryptSOPS`, restore it with `t.Cleanup`, record arguments,
return `errors.New("decrypt failed")`, and call `decryptSOPSFiles`. Assert:

- it passes the context, resolved compose paths, environment, and
  `sopsdecrypt.DefaultBinaryPath`;
- `errors.Is(err, exec.ErrDeployComposeFailure)` is true.

The patch-placement check in Step 5 will separately prove that both deployment
paths invoke the helper before their deployer.

Do not call `t.Parallel()` in tests that replace the package variable.

- [ ] **Step 2: Run command tests and verify they fail**

Run:

```bash
go run ./cmd/prepare
(cd .work/upstream/compose-unpacker && go test ./commands)
```

Expected: FAIL because `decryptSOPSFiles` does not exist.

- [ ] **Step 3: Implement the helper and minimal generated-tree edits**

Create `overlay/commands/sops.go`:

```go
var decryptSOPS = sopsdecrypt.Decrypt

func decryptSOPSFiles(ctx context.Context, composeFiles, env []string) error {
	if err := decryptSOPS(
		ctx,
		composeFiles,
		env,
		sopsdecrypt.DefaultBinaryPath,
	); err != nil {
		return fmt.Errorf("%w: decrypt SOPS files: %w", exec.ErrDeployComposeFailure, err)
	}
	return nil
}
```

After `composeFilePaths` is fully constructed and before logging/deployer
calls, add this to both generated command files:

```go
if err := decryptSOPSFiles(cmdCtx.Context, composeFilePaths, cmd.Env); err != nil {
	log.Error().Err(err).Msg("Failed to decrypt SOPS files")
	return err
}
```

In `compose_deploy.go`, replace:

```go
Strs("env", cmd.Env).
```

with:

```go
Int("envCount", len(cmd.Env)).
```

Keep the existing Swarm log free of environment values.

- [ ] **Step 4: Generate the persistent integration patch**

Generate the patch from the prepared compose-unpacker tree:

```bash
git -C .work/upstream/compose-unpacker diff -- \
  commands/compose_deploy.go commands/swarm_deploy.go \
  > patches/compose-unpacker.patch
test -s patches/compose-unpacker.patch
git -C .work/upstream/compose-unpacker reset --hard
go run ./cmd/prepare
```

Expected: the patch applies cleanly and the generated files contain both SOPS
calls and the redacted environment log.

- [ ] **Step 5: Validate the integrated upstream tree**

Run:

```bash
(cd .work/upstream/compose-unpacker && go test ./...)
(cd .work/upstream/compose-unpacker && go vet ./...)
test "$(grep -R 'Strs(\"env\"' .work/upstream/compose-unpacker/commands/compose_deploy.go | wc -l | tr -d ' ')" = "0"
python3 - <<'PY'
from pathlib import Path
for name in ("compose_deploy.go", "swarm_deploy.go"):
    source = Path(".work/upstream/compose-unpacker/commands", name).read_text()
    assert source.index("decryptSOPSFiles(") < source.index("deployer.Deploy(")
PY
git diff --check
```

Expected: all commands succeed.

- [ ] **Step 6: Commit upstream integration**

```bash
git add overlay/commands patches/compose-unpacker.patch
git commit -m "feat: decrypt SOPS files before stack deployment"
```

---

### Task 5: Verify SOPS and Build the Runtime Image

**Files:**
- Create: `internal/fetch/fetch.go`
- Create: `internal/fetch/fetch_test.go`
- Create: `cmd/fetch-sops/main.go`
- Create: `overlay/sopsdecrypt/integration_test.go`
- Create: `overlay/sopsdecrypt/testdata/age-key.txt`
- Create: `overlay/sopsdecrypt/testdata/config.sops.env`
- Create: `overlay/sopsdecrypt/testdata/config.env.expected`
- Create: `Dockerfile`
- Create: `.dockerignore`
- Replace: `Makefile`
- Create: `scripts/test-image.sh`

**Interfaces:**
- Consumes: `manifest.SOPS`
- Produces: `fetch.Download(ctx context.Context, client *http.Client, sourceURL, destination, expectedSHA256 string) error`
- Produces: `go run ./cmd/fetch-sops -manifest versions.json -output .work/dist/sops`
- Produces: `make image IMAGE=ghcr.io/jbruns/compose-unpacker:test`

- [ ] **Step 1: Write failing verified-download tests**

Use `httptest.Server` and cover success, non-2xx status, network failure,
checksum mismatch, destination mode, atomic replacement, and temporary cleanup.

```go
func TestDownloadRejectsChecksumMismatch(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "downloaded bytes")
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "sops")
	err := fetch.Download(
		context.Background(),
		server.Client(),
		server.URL,
		destination,
		strings.Repeat("0", 64),
	)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Download() error = %v", err)
	}
	if _, statErr := os.Stat(destination); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("destination exists after mismatch: %v", statErr)
	}
}
```

- [ ] **Step 2: Run fetch tests and verify they fail**

Run:

```bash
go test ./internal/fetch
```

Expected: FAIL because `fetch.Download` does not exist.

- [ ] **Step 3: Implement atomic checksum-verified download**

Stream the response through `io.MultiWriter(tempFile, sha256.New())`, require
HTTP 200, compare lowercase hex with `subtle.ConstantTimeCompare`, sync and
close, set mode `0755`, and atomically rename. Include URL host and destination
in errors but do not include response bodies.

Implement `cmd/fetch-sops` with `-manifest` and `-output` flags. It loads the
manifest and calls `fetch.Download` with a client timeout of two minutes.

- [ ] **Step 4: Add a real age integration fixture**

Generate a test-only age identity and encrypted non-sensitive fixture:

```bash
mkdir -p overlay/sopsdecrypt/testdata .work/tools
go run filippo.io/age/cmd/age-keygen@v1.3.1 \
  -o overlay/sopsdecrypt/testdata/age-key.txt
AGE_RECIPIENT="$(grep 'public key:' overlay/sopsdecrypt/testdata/age-key.txt | awk '{print $4}')"
printf 'DEMO_VALUE=decrypted-test-value\n' \
  > overlay/sopsdecrypt/testdata/config.env.expected
go run ./cmd/fetch-sops -output .work/tools/sops
SOPS_AGE_RECIPIENTS="$AGE_RECIPIENT" .work/tools/sops encrypt \
  --input-type dotenv \
  --output-type dotenv \
  overlay/sopsdecrypt/testdata/config.env.expected \
  > overlay/sopsdecrypt/testdata/config.sops.env
```

The key is intentionally committed test material and must contain a comment
stating that it protects no real secret.

Add an integration test with `//go:build integration` that reads the identity,
sets `SOPS_AGE_KEY`, copies the encrypted fixture under a temporary Compose
directory, calls `Decrypt` with `SOPS_BINARY`, and compares the generated
plaintext to `config.env.expected`. Add invalid-key and malformed-input
subtests and assert temporary files are removed.

- [ ] **Step 5: Run the real integration test**

Run:

```bash
go run ./cmd/prepare
go run ./cmd/fetch-sops -output .work/dist/sops
SOPS_BINARY="$PWD/.work/dist/sops" \
  bash -c 'cd .work/upstream/compose-unpacker && go test -tags=integration ./sopsdecrypt'
```

Expected: PASS for valid age input and PASS for the expected failure
assertions.

- [ ] **Step 6: Write the Dockerfile and Make targets**

Use the repository root as the build context. `.dockerignore` permits only the
prepared inputs needed by the Dockerfile:

```dockerfile
ARG GO_VERSION
ARG BASE_IMAGE
FROM golang:${GO_VERSION}-alpine AS builder
WORKDIR /src
COPY .work/upstream/portainer /src/portainer
COPY .work/upstream/compose-unpacker /src/compose-unpacker
WORKDIR /src/compose-unpacker
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/compose-unpacker .

FROM ${BASE_IMAGE}
ARG PORTAINER_VERSION
ARG SOPS_VERSION
ARG OVERLAY_REVISION
ARG SOURCE_REVISION
ARG BASE_DIGEST
COPY --from=builder /out/compose-unpacker /app/compose-unpacker
COPY .work/dist/sops /app/sops
LABEL org.opencontainers.image.source="https://github.com/jbruns/compose-unpacker" \
      org.opencontainers.image.revision="${SOURCE_REVISION}" \
      org.opencontainers.image.base.name="docker.io/portainer/compose-unpacker" \
      org.opencontainers.image.base.digest="${BASE_DIGEST}" \
      io.jbruns.portainer.version="${PORTAINER_VERSION}" \
      io.jbruns.sops.version="${SOPS_VERSION}" \
      io.jbruns.overlay.revision="${OVERLAY_REVISION}"
ENTRYPOINT ["/app/compose-unpacker"]
```

Create `cmd/manifest-value/main.go` as a strict accessor over `manifest.Load`.
It accepts exactly one of `go-version`, `lint-version`, `base-image`,
`base-digest`, `portainer-version`, `sops-version`, `overlay-revision`,
`immutable-tag`, or `version-tag`. Unknown fields fail.

Make `Makefile` read required values from `versions.json` through this command
rather than parsing JSON with shell regexes. Provide:

```text
make prepare
make test
make test-integration
make image IMAGE=ghcr.io/jbruns/compose-unpacker:test
make test-image IMAGE=ghcr.io/jbruns/compose-unpacker:test
make validate
make clean
```

`clean` may remove only `.work` and `coverage.out`.

Set root `.dockerignore` to exclude everything and then include only:

```text
**
!.work/
!.work/upstream/
!.work/upstream/compose-unpacker/**
!.work/upstream/portainer/**
!.work/dist/
!.work/dist/sops
```

- [ ] **Step 7: Add image smoke tests**

`scripts/test-image.sh` takes one image argument and verifies:

```bash
docker run --rm --entrypoint /app/compose-unpacker "$IMAGE" --help
docker run --rm --entrypoint /app/sops "$IMAGE" --version \
  | grep -F "sops 3.13.3"
test "$(docker image inspect "$IMAGE" --format '{{json .Config.Entrypoint}}')" \
  = '["/app/compose-unpacker"]'
test "$(docker image inspect "$IMAGE" --format '{{index .Config.Labels "io.jbruns.portainer.version"}}')" \
  = "2.45.0"
test "$(docker image inspect "$IMAGE" --format '{{index .Config.Labels "io.jbruns.sops.version"}}')" \
  = "v3.13.3"
```

Also inspect `org.opencontainers.image.base.digest` and compare it with the
manifest's pinned base reference.

- [ ] **Step 8: Build and test the image**

Run:

```bash
gofmt -w cmd/fetch-sops internal/fetch overlay/sopsdecrypt
go test ./internal/fetch
make image IMAGE=ghcr.io/jbruns/compose-unpacker:test
make test-image IMAGE=ghcr.io/jbruns/compose-unpacker:test
git diff --check
```

Expected: all commands succeed on Linux amd64 Docker.

- [ ] **Step 9: Commit image construction**

```bash
git add .dockerignore Dockerfile Makefile cmd/fetch-sops internal/fetch \
  overlay/sopsdecrypt scripts/test-image.sh
git commit -m "build: create verified SOPS runtime image"
```

---

### Task 6: Resolve Portainer LTS and SOPS Updates

**Files:**
- Create: `internal/update/resolver.go`
- Create: `internal/update/resolver_test.go`
- Create: `internal/update/github.go`
- Create: `internal/update/github_test.go`
- Create: `internal/update/image.go`
- Create: `internal/update/image_test.go`
- Create: `cmd/update-versions/main.go`
- Create: `cmd/update-versions/main_test.go`

**Interfaces:**
- Consumes: `manifest.Manifest`
- Produces: `update.Resolve(ctx context.Context, current manifest.Manifest, sources Sources) (manifest.Manifest, ChangeSummary, error)`
- Produces: `go run ./cmd/update-versions -manifest versions.json -write`
- Produces: `go run ./cmd/update-versions -manifest versions.json -check`

- [ ] **Step 1: Write failing release-selection tests**

Define:

```go
type Release struct {
	TagName    string
	Name       string
	Draft      bool
	Prerelease bool
}

type SOPSRelease struct {
	Version string
	Asset   string
	URL     string
	SHA256  string
}

type ChangeSummary struct {
	Changed          bool   `json:"changed"`
	PortainerBefore  string `json:"portainerBefore"`
	PortainerAfter   string `json:"portainerAfter"`
	SOPSBefore       string `json:"sopsBefore"`
	SOPSAfter        string `json:"sopsAfter"`
	OverlayRevision int    `json:"overlayRevision"`
}

type Sources interface {
	PortainerReleases(context.Context) ([]Release, error)
	TagCommit(context.Context, string, string) (string, error)
	LinuxAMD64Digest(context.Context, string, string) (string, error)
	LatestSOPS(context.Context) (SOPSRelease, error)
}
```

Cover:

- `2.45.0 LTS` beats a later-published `2.39.8 LTS`;
- STS, draft, prerelease, and malformed versions are ignored;
- no valid LTS is an error;
- new Portainer LTS resets overlay revision to `1`;
- SOPS-only update increments overlay revision;
- no change preserves the manifest byte-for-byte;
- missing matching compose-unpacker tag is an error;
- missing Linux amd64 image is an error; and
- missing SOPS checksum is an error.

Use this regression fixture:

```go
releases := []Release{
	{TagName: "2.39.8", Name: "Release 2.39.8 LTS"},
	{TagName: "2.45.0", Name: "Release 2.45.0 LTS"},
	{TagName: "2.46.0", Name: "Release 2.46.0 STS"},
}
```

- [ ] **Step 2: Run resolver tests and verify they fail**

Run:

```bash
go test ./internal/update
```

Expected: FAIL because the update package does not exist.

- [ ] **Step 3: Implement strict semantic release resolution**

Parse only `MAJOR.MINOR.PATCH` with three non-negative integer components.
Require `"LTS"` as a whitespace-delimited token in the release name. Compare
numeric components, not strings or publication order.

`Resolve` must:

1. select the highest Portainer LTS;
2. resolve both repository tag commits;
3. resolve the official Linux amd64 image digest;
4. resolve the latest stable SOPS asset and checksum;
5. reset revision for a Portainer change;
6. increment revision for a SOPS-only change; and
7. return a structured summary with old/new versions and immutable inputs.

- [ ] **Step 4: Write and implement GitHub client tests**

Use `httptest.Server` fixtures for:

```text
GET /repos/portainer/portainer/releases
GET /repos/portainer/portainer/git/ref/tags/2.45.0
GET /repos/portainer/compose-unpacker/git/ref/tags/2.45.0
GET /repos/getsops/sops/releases/latest
GET /sops-v3.13.3.checksums.txt
```

The client must set `Accept: application/vnd.github+json`,
`X-GitHub-Api-Version: 2022-11-28`, and optional bearer authentication from
`GITHUB_TOKEN`. Require commit refs; if a tag object is annotated, dereference
it through the Git tags API before returning the commit.

Parse the checksum file by exact asset-name field, reject duplicates, and
construct the asset URL from the release asset API response rather than string
concatenation.

- [ ] **Step 5: Write and implement image resolver tests**

Abstract command execution:

```go
type ImageInspector interface {
	Inspect(context.Context, string) ([]byte, error)
}
```

The production implementation runs:

```text
docker buildx imagetools inspect --format {{json .Manifest}} <image>:<tag>
```

Parse an OCI index, require exactly one `linux/amd64` manifest, and return its
`sha256:` digest. Tests cover absent, duplicate, malformed, and successful
manifests.

- [ ] **Step 6: Implement update CLI and deterministic writes**

`cmd/update-versions` supports:

```text
-manifest versions.json
-check
-write
```

The flags are mutually exclusive. `-check` exits `0` when current and `2` when
an update is available; `-write` writes formatted JSON to a same-directory
temporary file, syncs, and atomically renames it. Both modes print a JSON
`ChangeSummary` to stdout. API or validation errors exit `1`.

Extract `run(args []string, stdout, stderr io.Writer, sources Sources) int` and
test mutually exclusive flags, exit codes, no-change byte preservation,
atomic update output, and cleanup after a forced rename failure.

- [ ] **Step 7: Run resolver tests against current releases**

Run:

```bash
gofmt -w cmd/update-versions internal/update
go test -race ./internal/update
go vet ./internal/update
go run ./cmd/update-versions -manifest versions.json -check
test "$?" = "0"
git diff --check
```

Expected: tests pass and the live check reports no update beyond Portainer
2.45.0 and SOPS 3.13.3 at the implementation baseline. If a newer release
exists, update `versions.json` in the same task and rerun all prior validation
before committing.

- [ ] **Step 8: Commit update resolution**

```bash
git add cmd/update-versions internal/update versions.json
git commit -m "feat: resolve Portainer LTS and SOPS updates"
```

---

### Task 7: Add Reusable CI and Weekly Update Pull Requests

**Files:**
- Create: `scripts/validate.sh`
- Create: `.github/workflows/ci.yml`
- Create: `.github/workflows/update.yml`
- Create: `.github/dependabot.yml`
- Modify: `Makefile`

**Interfaces:**
- Consumes: all Make targets from Tasks 1-6
- Produces: `scripts/validate.sh [--image IMAGE]`
- Produces: reusable `ci.yml` through `workflow_call`
- Produces: weekly/manual update pull request

- [ ] **Step 1: Write the local validation script**

Use `set -euo pipefail` and run, in order:

```bash
go test -race ./internal/...
go vet ./internal/...
test -z "$(gofmt -l cmd internal overlay)"
find scripts -type f -name '*.sh' -print0 | xargs -0 -n1 bash -n
go run ./cmd/prepare
(cd .work/upstream/compose-unpacker && go test -race ./...)
(cd .work/upstream/compose-unpacker && go vet ./...)
go run ./cmd/fetch-sops -output .work/dist/sops
SOPS_BINARY="$PWD/.work/dist/sops" \
  bash -c 'cd .work/upstream/compose-unpacker && go test -tags=integration ./sopsdecrypt'
```

Then install `golangci-lint` at the manifest-pinned version into
`.work/bin` with:

```bash
GOBIN="$PWD/.work/bin" go install \
  github.com/golangci/golangci-lint/v2/cmd/golangci-lint@"$LINT_VERSION"
(
  cd .work/upstream/compose-unpacker
  "$OLDPWD/.work/bin/golangci-lint" run --timeout=10m -c .golangci.yaml ./...
)
```

If `--image` is supplied, build and smoke-test that image. The script must
print a stage heading before each command and stop on the first failure.

When `BASE_REF` is set, the script must also enforce release revision rules.
Use `git diff --name-only "$BASE_REF"...HEAD` and treat these as
release-impacting:

```text
versions.json
overlay/**
patches/**
Dockerfile
Makefile
scripts/**
cmd/fetch-sops/**
cmd/manifest-value/**
cmd/prepare/**
internal/fetch/**
internal/manifest/**
internal/prepare/**
```

Read the base manifest with
`git show "$BASE_REF:versions.json"` into a temporary file and query both
manifests through `cmd/manifest-value`. If a release-impacting path changed,
require revision `1` for a new Portainer version or a strictly larger revision
for the same Portainer version. Clean the temporary manifest with a trap.

- [ ] **Step 2: Run the validation script locally**

Run:

```bash
bash -n scripts/validate.sh
scripts/validate.sh --image ghcr.io/jbruns/compose-unpacker:test
```

Expected: all stages succeed.

- [ ] **Step 3: Create reusable pull-request CI**

Create `.github/workflows/ci.yml` with:

```yaml
on:
  pull_request:
  workflow_dispatch:
  workflow_call:

permissions:
  contents: read

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
          cache: true
      - uses: docker/setup-buildx-action@v3
      - run: scripts/validate.sh --image ghcr.io/jbruns/compose-unpacker:ci-${{ github.sha }}
        env:
          BASE_REF: ${{ github.event.pull_request.base.sha }}
```

Before committing, resolve each action's current major tag to a commit SHA and
pin `uses:` to that SHA with a trailing version comment. This keeps execution
immutable while allowing Dependabot to propose updates.

- [ ] **Step 4: Create the weekly update workflow**

Create `.github/workflows/update.yml` with:

```yaml
on:
  schedule:
    - cron: "17 9 * * 1"
  workflow_dispatch:

permissions:
  contents: write
  pull-requests: write

jobs:
  update:
    runs-on: ubuntu-latest
```

Steps:

1. checkout `main`;
2. set up Go and Buildx;
3. run `go run ./cmd/update-versions -manifest versions.json -write`;
4. stop successfully when `git diff --quiet -- versions.json`;
5. run `scripts/validate.sh --image ghcr.io/jbruns/compose-unpacker:update-${GITHUB_RUN_ID}`;
6. invoke `peter-evans/create-pull-request` pinned to a commit SHA;
7. use branch `automation/update-upstream`;
8. use commit `build: update Portainer and SOPS inputs`;
9. title the PR `build: update Portainer and SOPS inputs`; and
10. include the generated `ChangeSummary` and successful validation stages in
    the body.

Use only `GITHUB_TOKEN`; do not add a PAT or registry credentials. Because
GitHub suppresses ordinary workflow events caused by its own token, the update
workflow's full pre-PR validation is mandatory and must use the same
`scripts/validate.sh` entry point as PR CI.

- [ ] **Step 5: Configure Dependabot**

Create `.github/dependabot.yml`:

```yaml
version: 2
updates:
  - package-ecosystem: github-actions
    directory: /
    schedule:
      interval: weekly
    open-pull-requests-limit: 5
```

- [ ] **Step 6: Validate workflow syntax and behavior**

Run existing repository tooling only:

```bash
bash -n scripts/validate.sh
go test ./internal/...
scripts/validate.sh --image ghcr.io/jbruns/compose-unpacker:test
git diff --check
```

Use `gh workflow view` after pushing in the later publication step; do not add
a new YAML linter solely for this task.

- [ ] **Step 7: Commit CI and update automation**

```bash
git add .github Makefile scripts/validate.sh
git commit -m "ci: automate validation and upstream update PRs"
```

---

### Task 8: Publish Immutable GHCR Releases and Document Maintenance

**Files:**
- Create: `.github/workflows/release.yml`
- Replace: `README.md`
- Modify: `docs/superpowers/specs/2026-09-02-sops-compose-unpacker-maintenance-design.md` only if implementation discoveries require factual corrections

**Interfaces:**
- Consumes: `versions.json`, `Makefile`, `scripts/validate.sh`, `Dockerfile`
- Produces: `ghcr.io/jbruns/compose-unpacker:<portainer>-sops.<revision>`
- Produces: moving `<portainer>-sops` and `lts-sops` aliases

- [ ] **Step 1: Create the release workflow**

Trigger on pushes to `main` that change runtime, build, workflow, or manifest
files, plus manual dispatch:

```yaml
on:
  push:
    branches: [main]
    paths:
      - versions.json
      - overlay/**
      - patches/**
      - Dockerfile
      - Makefile
      - scripts/**
      - cmd/fetch-sops/**
      - cmd/manifest-value/**
      - cmd/prepare/**
      - internal/fetch/**
      - internal/manifest/**
      - internal/prepare/**
  workflow_dispatch:
```

Declare:

```yaml
permissions:
  contents: read
  packages: write
  id-token: write
  attestations: write
```

The job must:

1. checkout and set up Go/Buildx;
2. derive all three tags from validated `versions.json`;
3. log into `ghcr.io` with `${{ github.actor }}` and
   `${{ secrets.GITHUB_TOKEN }}`;
4. fail if `docker buildx imagetools inspect` finds the immutable tag;
5. run `scripts/validate.sh --image ...:prepublish-${GITHUB_SHA}`;
6. build and push the immutable and two moving tags with Buildx;
7. enable BuildKit SBOM and provenance attestations;
8. capture the pushed digest;
9. call `actions/attest-build-provenance` for the immutable image; and
10. print the immutable reference and digest in the job summary.

Pin every action to a commit SHA with a version comment.

- [ ] **Step 2: Add release-tag guard tests**

Extract tag derivation into the manifest package if it is not already covered.
Add tests:

```go
func TestReleaseTags(t *testing.T) {
	t.Parallel()
	m := validManifest()
	got := m.ReleaseTags("ghcr.io/jbruns/compose-unpacker")
	want := []string{
		"ghcr.io/jbruns/compose-unpacker:2.45.0-sops.1",
		"ghcr.io/jbruns/compose-unpacker:2.45.0-sops",
		"ghcr.io/jbruns/compose-unpacker:lts-sops",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ReleaseTags() = %v, want %v", got, want)
	}
}
```

Run:

```bash
go test ./internal/manifest
```

Expected: PASS.

- [ ] **Step 3: Replace README with user and maintainer documentation**

Document these exact topics:

- purpose and difference from vanilla Portainer;
- Linux amd64-only support;
- current Portainer and SOPS versions sourced from `versions.json`;
- immutable and moving image tags;
- recommended production use of the immutable tag;
- `SOPS_AGE_KEY` configuration through Portainer stack environment;
- recursive `*.sops.*` discovery below each Compose directory;
- `name.sops.ext` to `name.ext` mapping;
- fail-closed behavior and plaintext stack-directory lifecycle;
- local prerequisites: Go, Git, Docker Buildx, and network access;
- `make validate`, `make image`, and `make test-image`;
- weekly/manual update workflow and merge publication gate;
- overlay revision rules;
- the requirement that this repository use `main` as its default maintained
  branch before enabling scheduled updates and releases;
- repairing `patches/compose-unpacker.patch` after upstream changes;
- GHCR package visibility requirements; and
- verifying the GitHub provenance attestation and SBOM.

State that the historical manual notes are not current instructions. Do not
retain branch-wide import replacement, pseudo-version lookup, Docker Hub login,
or manual `build_and_push.sh` guidance.

- [ ] **Step 4: Run complete local release validation**

Run:

```bash
make clean
make validate IMAGE=ghcr.io/jbruns/compose-unpacker:test
go run ./cmd/update-versions -manifest versions.json -check
git diff --check
git status --short
```

Expected:

- all tests, lint, integration, image build, and smoke checks pass;
- update check exits `0` with no pending update at the implementation
  baseline; and
- only intentional documentation/workflow changes are uncommitted.

- [ ] **Step 5: Review the complete change set**

Run the `requesting-code-review` skill. Address only high-confidence findings
that relate to this specification, rerun `make validate`, and inspect:

```bash
git --no-pager diff --stat 4ca7e1b..HEAD
git --no-pager diff --check 4ca7e1b..HEAD
```

Expected: no unresolved correctness or security findings.

- [ ] **Step 6: Commit release automation and documentation**

```bash
git add .github/workflows/release.yml README.md internal/manifest \
  docs/superpowers/specs/2026-09-02-sops-compose-unpacker-maintenance-design.md
git commit -m "docs: document SOPS image release maintenance"
```

- [ ] **Step 7: Verify the final branch**

Invoke the `verification-before-completion` skill, then run:

```bash
make clean
make validate IMAGE=ghcr.io/jbruns/compose-unpacker:test
git status --short
git --no-pager log --oneline 4ca7e1b..HEAD
```

Expected: validation succeeds, the worktree is clean, and task commits are
listed in order.

- [ ] **Step 8: Prepare publication without pushing**

Use the `finishing-a-development-branch` skill to present merge/push/PR
options. Do not push, publish, or create a pull request without explicit user
approval. The publication handoff must include creating or selecting `main`,
merging the implementation there, setting `main` as the GitHub default branch,
making the GHCR package public, and then enabling the scheduled workflows.
