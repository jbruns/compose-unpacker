#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
WORKFLOW="$ROOT/.github/workflows/release.yml"
SCRATCH="$ROOT/.tmp/test-release-workflow.$$"
OUTPUT="$SCRATCH/output"
GITHUB_OUTPUT_FILE="$SCRATCH/github-output"
METADATA_SCRIPT="$SCRATCH/metadata.sh"
BRANCH_GUARD_SCRIPT="$SCRATCH/branch-guard.sh"
GUARD_SCRIPT="$SCRATCH/guard.sh"
PROMOTE_SCRIPT="$SCRATCH/promote.sh"
DOCKER_LOG="$SCRATCH/docker.log"
FAKE_BIN="$SCRATCH/bin"

cleanup() {
	rm -rf "$SCRATCH"
}
trap cleanup EXIT

fail() {
	cat "$OUTPUT" >&2 2>/dev/null || true
	echo "$*" >&2
	exit 1
}

[[ -f "$WORKFLOW" ]] || fail "release workflow is missing"

rm -rf "$SCRATCH"
mkdir -p "$FAKE_BIN"
: >"$OUTPUT"

ruby -rpsych -e '
  workflow = Psych.safe_load(File.read(ARGV.fetch(0)), aliases: true)
  triggers = workflow.fetch(true)
  push = triggers.fetch("push")
  abort "release push branch must be main" unless push.fetch("branches") == ["main"]
  expected_paths = %w[
    versions.json overlay/** patches/** Dockerfile Makefile scripts/**
    cmd/fetch-sops/** cmd/manifest-value/** cmd/prepare/**
    internal/fetch/** internal/manifest/** internal/prepare/**
  ]
  abort "release paths differ from the release-impacting set" unless push.fetch("paths") == expected_paths
  abort "workflow_dispatch trigger is missing" unless triggers.key?("workflow_dispatch")

  expected_permissions = {
    "contents" => "read",
    "packages" => "write",
    "id-token" => "write",
    "attestations" => "write",
  }
  abort "release permissions must not be workflow-wide" if workflow.key?("permissions")
  release_job = workflow.fetch("jobs").fetch("release")
  abort "release job permissions are not least privilege" unless release_job.fetch("permissions") == expected_permissions
  expected_concurrency = {
    "group" => "release-${{ github.repository }}",
    "cancel-in-progress" => false,
  }
  abort "release runs are not serialized" unless workflow.fetch("concurrency") == expected_concurrency

  steps = release_job.fetch("steps")
  find_step = ->(name) {
    steps.find { |step| step["name"] == name } or abort "#{name} step not found"
  }
  branch_guard = find_step.call("Require main branch")
  abort "main-branch guard must be the first release step" unless steps.first == branch_guard
  login = find_step.call("Log in to GHCR").fetch("with")
  abort "GHCR registry login is missing" unless login.fetch("registry") == "ghcr.io"
  abort "GHCR actor login is missing" unless login.fetch("username") == "${{ github.actor }}"
  abort "GITHUB_TOKEN login is missing" unless login.fetch("password") == "${{ secrets.GITHUB_TOKEN }}"

  build = find_step.call("Build candidate artifact")
  build_with = build.fetch("with")
  abort "candidate artifact is not pushed" unless build_with.fetch("push") == true
  abort "release platform is wrong" unless build_with.fetch("platforms") == "linux/amd64"
  abort "candidate build publishes release tags" unless build_with.fetch("tags") == "${{ steps.release.outputs.candidate }}"
  abort "SBOM is disabled" unless build_with.fetch("sbom") == true
  abort "BuildKit provenance is disabled" unless build_with.fetch("provenance") == "mode=max"
  %w[
    GO_VERSION BASE_IMAGE BASE_DIGEST PORTAINER_VERSION SOPS_VERSION
    OVERLAY_REVISION COMPOSE_UNPACKER_COMMIT PORTAINER_SERVER_COMMIT
    SOURCE_REVISION BUILD_CREATED
  ].each do |arg|
    abort "missing #{arg} build argument" unless build_with.fetch("build-args").include?("#{arg}=")
  end

  validate = find_step.call("Validate candidate artifact")
  abort "candidate validation does not reuse the built artifact" unless validate.fetch("run") ==
    "scripts/validate.sh --existing-image \"$CANDIDATE_REFERENCE\""
  abort "candidate validation is not digest-pinned" unless validate.fetch("env").fetch("CANDIDATE_REFERENCE") ==
    "${{ steps.release.outputs.candidate }}@${{ steps.build.outputs.digest }}"

  promote = find_step.call("Promote validated artifact")
  promote_env = promote.fetch("env")
  abort "promotion source is not the validated digest" unless promote_env.fetch("CANDIDATE_REFERENCE") ==
    "${{ steps.release.outputs.candidate }}@${{ steps.build.outputs.digest }}"
  abort "promotion digest is not the validated digest" unless promote_env.fetch("VALIDATED_DIGEST") ==
    "${{ steps.build.outputs.digest }}"
  abort "release was promoted before validation" unless steps.index(build) < steps.index(validate) &&
    steps.index(validate) < steps.index(promote)

  attest = find_step.call("Attest immutable image").fetch("with")
  abort "attestation subject is wrong" unless attest.fetch("subject-name") == "ghcr.io/jbruns/compose-unpacker"
  abort "attestation digest is not the validated digest" unless attest.fetch("subject-digest") ==
    "${{ steps.build.outputs.digest }}"
  abort "registry attestation is not pushed" unless attest.fetch("push-to-registry") == true

  print find_step.call("Derive release metadata").fetch("run")
  File.write(ARGV.fetch(1), branch_guard.fetch("run"))
  File.write(ARGV.fetch(2), find_step.call("Reject existing immutable tag").fetch("run"))
  File.write(ARGV.fetch(3), promote.fetch("run"))
' "$WORKFLOW" "$BRANCH_GUARD_SCRIPT" "$GUARD_SCRIPT" "$PROMOTE_SCRIPT" >"$METADATA_SCRIPT"

if grep -E '^[[:space:]]*uses:' "$WORKFLOW" |
	grep -Ev 'uses: [^@[:space:]]+@[0-9a-f]{40}[[:space:]]+# v[0-9]'; then
	fail "every action must be SHA-pinned with a version comment"
fi

cat >"$FAKE_BIN/go" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
field=${*: -1}
if [[ "${FAIL_ACCESSOR:-}" == "$field" ]]; then
	echo "forced accessor failure: $field" >&2
	exit 42
fi
case "$field" in
	release-tags)
		repository=
		while (($#)); do
			if [[ "$1" == -repository ]]; then
				repository=$2
				break
			fi
			shift
		done
		printf '%s\n' \
			"${repository}:2.45.0-sops.1" \
			"${repository}:2.45.0-sops" \
			"${repository}:lts-sops"
		;;
	go-version) echo 1.26.6 ;;
	base-image) echo docker.io/portainer/compose-unpacker@sha256:base ;;
	base-digest) echo sha256:base ;;
	portainer-version) echo 2.45.0 ;;
	compose-unpacker-commit) echo 23c8e42176c521cb6745b3ea95233d3a68bbe031 ;;
	portainer-server-commit) echo d79ba726cd54395a54cca5e9180609ce52fa7a4f ;;
	sops-version) echo v3.13.3 ;;
	overlay-revision) echo 1 ;;
	*) echo "unexpected go arguments: $*" >&2; exit 64 ;;
esac
EOF

cat >"$FAKE_BIN/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ -n "${DOCKER_LOG:-}" ]]; then
	printf '%s\n' "$*" >>"$DOCKER_LOG"
fi

if [[ "$1 $2 $3" == "buildx imagetools create" ]]; then
	exit 0
fi
if [[ "$1 $2 $3" != "buildx imagetools inspect" ]]; then
	exit 64
fi
if [[ $# -eq 6 && "$5" == --format ]]; then
	echo "${PROMOTED_DIGEST:-sha256:validated}"
	exit 0
fi
[[ $# -eq 4 && "$4" == "ghcr.io/jbruns/compose-unpacker:2.45.0-sops.1" ]] ||
	exit 64
case "${INSPECT_RESULT:-absent}" in
	absent) echo "ERROR: $4: not found" >&2; exit 1 ;;
	manifest-unknown) echo "ERROR: $4: manifest unknown" >&2; exit 1 ;;
	exists) exit 0 ;;
	error) echo 'registry connection failed' >&2; exit 42 ;;
	credential-error)
		echo 'error getting credentials: credential helper not found' >&2
		echo "ERROR: $4: not found" >&2
		exit 1
		;;
	tool-error)
		echo "buildx plugin error: $4: manifest unknown" >&2
		exit 1
		;;
	network-error)
		echo "network lookup failed: manifest unknown: $4" >&2
		exit 1
		;;
	*) exit 64 ;;
esac
EOF

cat >"$FAKE_BIN/git" <<'EOF'
#!/bin/sh
if [ "$*" = "show -s --format=%cI aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" ]; then
	echo 2026-09-02T23:41:46+00:00
	exit 0
fi
echo "unexpected git arguments: $*" >&2
exit 64
EOF
chmod +x "$FAKE_BIN/go" "$FAKE_BIN/docker" "$FAKE_BIN/git"

GITHUB_REF=refs/heads/main bash "$BRANCH_GUARD_SCRIPT" >"$OUTPUT" 2>&1 ||
	fail "main branch was rejected"

set +e
GITHUB_REF=refs/heads/release-candidate \
	bash "$BRANCH_GUARD_SCRIPT" >"$OUTPUT" 2>&1
status=$?
set -e
[[ $status -ne 0 ]] || fail "non-main manual dispatch was accepted"
grep -F 'releases are restricted to refs/heads/main' "$OUTPUT" >/dev/null ||
	fail "non-main manual dispatch did not explain the branch restriction"

(
	cd "$ROOT"
	GITHUB_OUTPUT="$GITHUB_OUTPUT_FILE" \
		GITHUB_SHA=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
		IMAGE_REPOSITORY=ghcr.io/jbruns/compose-unpacker \
		PATH="$FAKE_BIN:$PATH" \
		bash "$METADATA_SCRIPT"
) >"$OUTPUT" 2>&1 || fail "release metadata step failed"

for expected in \
	'immutable=ghcr.io/jbruns/compose-unpacker:2.45.0-sops.1' \
	'version=ghcr.io/jbruns/compose-unpacker:2.45.0-sops' \
	'lts=ghcr.io/jbruns/compose-unpacker:lts-sops' \
	'candidate=ghcr.io/jbruns/compose-unpacker:prepublish-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' \
	'go-version=1.26.6' \
	'base-image=docker.io/portainer/compose-unpacker@sha256:base' \
	'base-digest=sha256:base' \
	'portainer-version=2.45.0' \
	'compose-unpacker-commit=23c8e42176c521cb6745b3ea95233d3a68bbe031' \
	'portainer-server-commit=d79ba726cd54395a54cca5e9180609ce52fa7a4f' \
	'sops-version=v3.13.3' \
	'overlay-revision=1' \
	'build-created=2026-09-02T23:41:46+00:00'; do
	grep -Fx "$expected" "$GITHUB_OUTPUT_FILE" >/dev/null ||
		fail "release metadata output is missing: $expected"
done

: >"$GITHUB_OUTPUT_FILE"
set +e
(
	cd "$ROOT"
	FAIL_ACCESSOR=sops-version \
		GITHUB_OUTPUT="$GITHUB_OUTPUT_FILE" \
		GITHUB_SHA=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
		IMAGE_REPOSITORY=ghcr.io/jbruns/compose-unpacker \
		PATH="$FAKE_BIN:$PATH" \
		bash "$METADATA_SCRIPT"
) >"$OUTPUT" 2>&1
status=$?
set -e
[[ $status -ne 0 ]] || fail "release metadata hid a manifest accessor failure"
[[ ! -s "$GITHUB_OUTPUT_FILE" ]] ||
	fail "release metadata emitted partial outputs after an accessor failure"

IMMUTABLE_TAG=ghcr.io/jbruns/compose-unpacker:2.45.0-sops.1 \
	PATH="$FAKE_BIN:$PATH" \
	bash "$GUARD_SCRIPT" >"$OUTPUT" 2>&1 ||
	fail "immutable guard rejected an absent tag"

set +e
IMMUTABLE_TAG=ghcr.io/jbruns/compose-unpacker:2.45.0-sops.1 \
	INSPECT_RESULT=exists \
	PATH="$FAKE_BIN:$PATH" \
	bash "$GUARD_SCRIPT" >"$OUTPUT" 2>&1
status=$?
set -e
[[ $status -ne 0 ]] || fail "immutable guard accepted an existing tag"
grep -F 'immutable tag already exists' "$OUTPUT" >/dev/null ||
	fail "immutable guard did not explain the collision"

set +e
IMMUTABLE_TAG=ghcr.io/jbruns/compose-unpacker:2.45.0-sops.1 \
	INSPECT_RESULT=error \
	PATH="$FAKE_BIN:$PATH" \
	bash "$GUARD_SCRIPT" >"$OUTPUT" 2>&1
status=$?
set -e
[[ $status -ne 0 ]] || fail "immutable guard accepted an inconclusive registry error"

IMMUTABLE_TAG=ghcr.io/jbruns/compose-unpacker:2.45.0-sops.1 \
	INSPECT_RESULT=manifest-unknown \
	PATH="$FAKE_BIN:$PATH" \
	bash "$GUARD_SCRIPT" >"$OUTPUT" 2>&1 ||
	fail "immutable guard rejected a manifest-unknown response tied to the immutable reference"

for inspect_result in credential-error tool-error network-error; do
	set +e
	IMMUTABLE_TAG=ghcr.io/jbruns/compose-unpacker:2.45.0-sops.1 \
		INSPECT_RESULT=$inspect_result \
		PATH="$FAKE_BIN:$PATH" \
		bash "$GUARD_SCRIPT" >"$OUTPUT" 2>&1
	status=$?
	set -e
	[[ $status -ne 0 ]] ||
		fail "immutable guard accepted $inspect_result containing not found"
done

: >"$DOCKER_LOG"
CANDIDATE_REFERENCE=ghcr.io/jbruns/compose-unpacker:prepublish-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa@sha256:validated \
	VALIDATED_DIGEST=sha256:validated \
	IMMUTABLE_TAG=ghcr.io/jbruns/compose-unpacker:2.45.0-sops.1 \
	VERSION_TAG=ghcr.io/jbruns/compose-unpacker:2.45.0-sops \
	LTS_TAG=ghcr.io/jbruns/compose-unpacker:lts-sops \
	DOCKER_LOG="$DOCKER_LOG" \
	PATH="$FAKE_BIN:$PATH" \
	bash "$PROMOTE_SCRIPT" >"$OUTPUT" 2>&1 ||
	fail "validated candidate promotion failed"
grep -Fx \
	'buildx imagetools create --tag ghcr.io/jbruns/compose-unpacker:2.45.0-sops.1 --tag ghcr.io/jbruns/compose-unpacker:2.45.0-sops --tag ghcr.io/jbruns/compose-unpacker:lts-sops ghcr.io/jbruns/compose-unpacker:prepublish-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa@sha256:validated' \
	"$DOCKER_LOG" >/dev/null ||
	fail "release tags were not promoted from the validated digest"
for tag in \
	ghcr.io/jbruns/compose-unpacker:2.45.0-sops.1 \
	ghcr.io/jbruns/compose-unpacker:2.45.0-sops \
	ghcr.io/jbruns/compose-unpacker:lts-sops; do
	grep -Fx "buildx imagetools inspect $tag --format {{.Manifest.Digest}}" "$DOCKER_LOG" >/dev/null ||
		fail "promoted tag digest was not verified: $tag"
done

set +e
CANDIDATE_REFERENCE=ghcr.io/jbruns/compose-unpacker:prepublish-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa@sha256:validated \
	VALIDATED_DIGEST=sha256:validated \
	IMMUTABLE_TAG=ghcr.io/jbruns/compose-unpacker:2.45.0-sops.1 \
	VERSION_TAG=ghcr.io/jbruns/compose-unpacker:2.45.0-sops \
	LTS_TAG=ghcr.io/jbruns/compose-unpacker:lts-sops \
	PROMOTED_DIGEST=sha256:wrong \
	PATH="$FAKE_BIN:$PATH" \
	bash "$PROMOTE_SCRIPT" >"$OUTPUT" 2>&1
status=$?
set -e
[[ $status -ne 0 ]] || fail "promotion accepted a release tag with the wrong digest"
grep -F "does not reference validated digest" "$OUTPUT" >/dev/null ||
	fail "promotion digest mismatch did not explain the failure"
