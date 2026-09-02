# SOPS Compose Unpacker Maintenance Design

## Summary

This project will stop maintaining a source fork of Portainer's
`compose-unpacker`. Instead, it will maintain a small, tested overlay that is
applied to immutable upstream sources for a specific Portainer LTS release.

The first supported release will use Portainer 2.45.0 and publish a Linux
amd64 image to GitHub Container Registry (GHCR). A weekly workflow will detect
new Portainer LTS and SOPS releases, validate an update, and open a pull
request. Merging the pull request will be the only automatic publication gate.

## Goals

- Add automatic SOPS decryption to the vanilla Portainer compose-unpacker
  behavior for Compose and Swarm deployments.
- Build the first maintained image from Portainer LTS 2.45.0.
- Make future LTS updates reviewable and mostly automatic.
- Preserve the matching official Portainer image as the runtime base.
- Publish reproducible Linux amd64 images to GHCR without long-lived registry
  credentials.
- Exercise decryption with an actual SOPS binary and an age key in CI.
- Fail closed without exposing key material, environment values, or decrypted
  content in logs.

## Non-goals

- Supporting Linux arm, Linux arm64, or Windows images.
- Automatically publishing an upstream update without review and merge.
- Maintaining a full downstream copy of every upstream commit.
- Adding a general-purpose pre-deployment plugin system.
- Supporting arbitrary encrypted-file discovery rules in the first release.
- Depending on Portainer accepting this feature upstream.

An upstream contribution may be pursued separately, but this project must
remain maintainable without it.

## Validated Upstream State

The design is based on the following observations, revalidated on
2026-09-02:

- Portainer 2.45.0 is published as an LTS release.
- `portainer/compose-unpacker` has a matching `2.45.0` tag.
- `portainer/compose-unpacker:2.45.0` includes a Linux amd64 image.
- The compose-unpacker source changed substantially between 2.26 and 2.45.0,
  including splitting deployment commands into a `commands` package.
- The 2.45.0 compose-unpacker `go.mod` contains a relative replacement for
  `github.com/portainer/portainer`, so a standalone build must provide and
  reference the matching Portainer source tree.
- SOPS v3.13.3 is the current stable release and publishes a Linux amd64
  binary with checksums.
- The legacy fork's SOPS version, ARM assumptions, source locations, and
  manual dependency update process are stale.

These values are initial inputs, not permanent assumptions. The input manifest
and update automation described below are authoritative.

## Architecture

### Repository ownership boundary

The repository will own only:

- an immutable `versions.json` input manifest;
- SOPS decryption source under an `overlay` tree and a minimal integration
  patch;
- tests and encrypted fixtures;
- deterministic source preparation and build scripts;
- a Linux amd64 multi-stage Dockerfile;
- Make targets;
- GitHub update, CI, and release workflows;
- Dependabot configuration; and
- user and maintainer documentation.

It will not carry the complete Portainer source history on its maintained
branch. Generated upstream checkouts, build contexts, binaries, decrypted test
outputs, and image metadata files will be ignored.

### Immutable input manifest

One reviewed `versions.json` manifest will record:

- Portainer release version;
- exact `portainer/compose-unpacker` commit;
- exact `portainer/portainer` commit;
- official vanilla compose-unpacker image reference and Linux amd64 digest;
- SOPS version;
- SOPS Linux amd64 asset name, URL, and SHA-256 checksum; and
- downstream overlay revision.

The build must consume resolved commits and digests from this manifest rather
than mutable branches or unqualified image tags. Preparation must verify that
the recorded commits are reachable from the matching upstream tags.

The overlay revision starts at `1` for each Portainer version. Any published
downstream code or build change for the same Portainer version increments it.

### Source preparation

A single preparation command will:

1. create a clean generated workspace;
2. fetch the compose-unpacker and Portainer source trees at their recorded
   commits;
3. verify the corresponding release tags;
4. rewrite compose-unpacker's relative Portainer module replacement to the
   generated Portainer checkout;
5. copy new source and test files from the repository's `overlay` tree,
   refusing to overwrite an upstream file;
6. check the upstream integration edits with `git apply --check`;
7. apply the integration patch; and
8. verify the resulting tree differs from upstream only by the copied overlay,
   the integration patch, and the local module replacement.

The command must be idempotent and fail with a stage-specific error. It must
not infer a Portainer pseudo-version or rewrite module import paths.

### Image build

The builder stage compiles the patched compose-unpacker binary from the
prepared sources. The final stage uses the matching official
`portainer/compose-unpacker` image pinned by the manifest's digest. It replaces
only `/app/compose-unpacker` and adds the checksum-verified SOPS binary.

The resulting image retains the vanilla image's runtime filesystem and
entrypoint. OCI labels will identify:

- the upstream Portainer version and commits;
- the SOPS version;
- the downstream repository revision; and
- the build timestamp and source URL.

## Runtime Decryption

### Invocation point

Compose and Swarm deployment commands will call a shared decryption component
after the repository is cloned and compose file paths are resolved, but before
the compose files are parsed or deployment starts.

The component receives:

- the deployment context;
- resolved compose file paths; and
- the deployment environment.

Context cancellation must terminate an in-progress SOPS process.

### File discovery

For each compose file, the component uses its containing directory as a scan
root. It removes roots contained by another root using path-aware relative
checks rather than string-prefix comparisons. For example, `/a/b` must not be
treated as the parent of `/a/bb`.

Each unique root is walked recursively with these rules:

- do not follow symbolic links;
- skip `.git` directories;
- select regular files whose base name matches `*.sops.*`;
- sort selected paths for deterministic processing; and
- propagate every traversal error.

The destination name is produced by replacing the first `.sops.` marker in
the base name with `.`. Thus, `service.sops.env` becomes `service.env`.
Multiple encrypted inputs that resolve to the same destination are an error.

### Decryption and output

SOPS runs by absolute path with the deployment context and environment.
Supplying `SOPS_AGE_KEY` through the deployment environment is the documented
age-key mechanism.

For each encrypted file:

1. create a restrictive temporary file in the destination directory;
2. stream decrypted output directly to that file;
3. close and synchronize the file successfully;
4. atomically replace the destination; and
5. remove temporary output on every failure path.

An existing destination from an earlier keep/redeploy operation may be
replaced. The operation must replace a destination symlink itself rather than
follow it.

Any discovery, process, write, synchronization, or rename failure aborts the
deployment before Compose or Swarm deployment starts. Error messages may name
the encrypted file and failed stage, but must not include the SOPS environment,
age key, decrypted bytes, or full command environment.

Plaintext outputs remain in the cloned stack directory because Compose may
need them for `env_file`, configs, or bind-mounted assets after parsing. Their
lifecycle continues to follow Portainer's stack-directory lifecycle.

### Environment logging

The upstream Compose command currently logs raw environment values at info
level. The overlay will replace this with non-sensitive metadata, such as the
environment entry count. Neither Compose nor Swarm paths may log raw
deployment environment values.

## Update Automation

### Schedule and manual execution

An update workflow runs weekly and supports `workflow_dispatch`. It checks
Portainer and SOPS independently so a SOPS update does not wait for a new
Portainer LTS.

### Portainer LTS selection

The updater queries non-draft, non-prerelease Portainer GitHub releases,
retains releases explicitly labeled `LTS`, validates semantic version tags,
and selects the highest semantic version. It must not select only by publish
time because maintenance releases for older LTS lines may be published later.

For a candidate, it verifies:

- matching version tags exist in both Portainer repositories;
- both tags resolve to commits;
- the official compose-unpacker image exists; and
- the image index contains Linux amd64.

It then records the exact commits and Linux amd64 image digest.

### SOPS selection

The updater selects the latest stable SOPS GitHub release, requires the
expected Linux amd64 asset and published checksum, and records the resolved
asset URL and checksum. Missing or ambiguous release assets are hard failures.

### Update pull request

When any input changes, the workflow creates or refreshes one bot branch. It
updates the manifest, resets the overlay revision to `1` for a new Portainer
version, and runs the complete validation suite before opening the pull
request.

The pull request reports:

- old and new versions;
- resolved commits and image digest;
- SOPS asset checksum;
- validation stages and results; and
- an explicit patch incompatibility if the overlay no longer applies.

No update failure publishes an image. A failed scheduled run remains visible
through GitHub Actions notifications and logs.

## CI and Release

### Pull request CI

Every pull request runs:

- manifest schema and upstream-reference verification;
- SOPS checksum verification;
- clean source preparation and patch application;
- `gofmt` checks for owned Go source and `bash -n` checks for owned shell
  source;
- `go vet ./...` and `go test ./...` in the prepared compose-unpacker tree;
- upstream `golangci-lint` using its checked-in configuration and the matching
  Portainer checkout's pinned linter version;
- the real age/SOPS integration test;
- a Linux amd64 image build;
- `compose-unpacker --help`;
- `sops --version`; and
- image configuration and label inspection.

The update workflow runs the same validation before opening its pull request.

### Release gate

Only a merge to `main` may publish. The release workflow reruns the complete
suite, authenticates to GHCR with GitHub's short-lived `GITHUB_TOKEN`, and
publishes:

- immutable:
  `<portainer-version>-sops.<overlay-revision>`;
- moving within the same Portainer version:
  `<portainer-version>-sops`; and
- moving to the current supported LTS:
  `lts-sops`.

For example, the initial immutable tag is
`ghcr.io/jbruns/compose-unpacker:2.45.0-sops.1`.

The workflow must refuse to overwrite an existing immutable tag. It emits an
SBOM and GitHub build-provenance attestation for the immutable image digest.
Workflow permissions will be declared per job and limited to the operations
each job needs.

GitHub Actions dependencies will be maintained by Dependabot.

## Testing Strategy

### Unit tests

Owned Go tests will cover:

- compose-directory root derivation and deduplication;
- path-boundary cases such as `/a/b` and `/a/bb`;
- `.git` exclusion and non-followed symlinks;
- deterministic file ordering;
- encrypted-to-plaintext name mapping;
- duplicate output rejection;
- traversal-error propagation;
- context cancellation;
- decrypt-process failure;
- temporary-file cleanup;
- restrictive output permissions;
- atomic replacement of an existing output; and
- environment and error redaction.

Where process behavior is not under test, a narrow command-runner interface
will allow deterministic fakes. Production code will still execute the real
SOPS binary directly.

### Integration tests

The repository will contain a non-secret age-encrypted fixture and a test-only
age private key. CI will use the pinned SOPS binary with `SOPS_AGE_KEY` to
decrypt the fixture and compare the result with expected non-sensitive test
content.

Negative integration cases will include:

- invalid age key;
- malformed encrypted input; and
- a forced destination failure.

Tests must clean plaintext and temporary output. Neither generated plaintext
nor the test key may appear in production image layers.

### Image smoke tests

The built Linux amd64 image must:

- retain `/app/compose-unpacker` as its entrypoint;
- execute `compose-unpacker --help`;
- execute the expected `sops --version`;
- contain the expected provenance labels; and
- derive from the manifest-pinned vanilla image digest.

## Documentation

The README will document:

- the project's downstream-overlay model;
- supported platform and Portainer/SOPS versions;
- image names and immutable versus moving tags;
- use of `SOPS_AGE_KEY`;
- the `*.sops.*` to plaintext naming convention;
- scan boundaries and fail-closed behavior;
- local prepare, test, and image-build commands;
- the weekly update pull-request flow;
- how to increment the overlay revision;
- how to repair an overlay after upstream refactoring; and
- how to verify image provenance.

The manual notes are historical input only. Instructions that require creating
release branches, replacing module paths, manually finding pseudo-version
commits, or limiting builds because old SOPS lacked an architecture must not
remain as current maintenance guidance.

## Failure Handling

Failures must identify their stage and preserve the previous published image:

- unresolved or inconsistent upstream release: stop before changing manifest;
- missing image platform or SOPS asset/checksum: stop before opening a PR;
- overlay conflict: open no release and report the failed patch;
- compile, test, or smoke failure: open no release and retain logs/artifacts;
- existing immutable image tag: fail publication rather than overwrite it; and
- runtime scan or decryption failure: abort before stack deployment.

No fallback may silently use a mutable tag, a different upstream commit, an
unverified SOPS binary, or the previous plaintext output.

## Migration

The existing 2.26-based fork commits will not be rebased onto 2.45.0. The new
maintained branch will introduce the overlay structure and produce a clean
2.45.0-based image. Existing Docker Hub images remain untouched but are not
updated by the new workflow.

The initial release is complete when:

1. all validation described above passes for Portainer 2.45.0;
2. the immutable GHCR image is published with an SBOM and provenance
   attestation;
3. `2.45.0-sops` and `lts-sops` resolve to that digest;
4. documentation contains no manual branch/dependency reconstruction steps;
   and
5. a dry run of the updater reports no pending Portainer or SOPS update.
