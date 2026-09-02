#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
WORKFLOW="$ROOT/.github/workflows/release.yml"
SCRATCH="$ROOT/.tmp/test-release-workflow.$$"
OUTPUT="$SCRATCH/output"
GITHUB_OUTPUT_FILE="$SCRATCH/github-output"
METADATA_SCRIPT="$SCRATCH/metadata.sh"
GUARD_SCRIPT="$SCRATCH/guard.sh"
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
  abort "release permissions are not least privilege" unless workflow.fetch("permissions") == expected_permissions
  expected_concurrency = {
    "group" => "release-${{ github.repository }}",
    "cancel-in-progress" => false,
  }
  abort "release runs are not serialized" unless workflow.fetch("concurrency") == expected_concurrency

  steps = workflow.fetch("jobs").fetch("release").fetch("steps")
  find_step = ->(name) {
    steps.find { |step| step["name"] == name } or abort "#{name} step not found"
  }
  login = find_step.call("Log in to GHCR").fetch("with")
  abort "GHCR registry login is missing" unless login.fetch("registry") == "ghcr.io"
  abort "GHCR actor login is missing" unless login.fetch("username") == "${{ github.actor }}"
  abort "GITHUB_TOKEN login is missing" unless login.fetch("password") == "${{ secrets.GITHUB_TOKEN }}"

  validate = find_step.call("Validate prepublish image")
  abort "prepublish validation is incomplete" unless validate.fetch("run") == "scripts/validate.sh --image \"$PREPUBLISH_IMAGE\""
  abort "prepublish tag is wrong" unless validate.fetch("env").fetch("PREPUBLISH_IMAGE") ==
    "ghcr.io/jbruns/compose-unpacker:prepublish-${{ github.sha }}"

  build = find_step.call("Build and push release")
  build_with = build.fetch("with")
  abort "release does not push" unless build_with.fetch("push") == true
  abort "release platform is wrong" unless build_with.fetch("platforms") == "linux/amd64"
  abort "release tags do not come from metadata" unless build_with.fetch("tags") == "${{ steps.release.outputs.tags }}"
  abort "SBOM is disabled" unless build_with.fetch("sbom") == true
  abort "BuildKit provenance is disabled" unless build_with.fetch("provenance") == "mode=max"
  %w[GO_VERSION BASE_IMAGE BASE_DIGEST PORTAINER_VERSION SOPS_VERSION OVERLAY_REVISION SOURCE_REVISION].each do |arg|
    abort "missing #{arg} build argument" unless build_with.fetch("build-args").include?("#{arg}=")
  end

  attest = find_step.call("Attest immutable image").fetch("with")
  abort "attestation subject is wrong" unless attest.fetch("subject-name") == "ghcr.io/jbruns/compose-unpacker"
  abort "attestation digest is not the pushed digest" unless attest.fetch("subject-digest") ==
    "${{ steps.push.outputs.digest }}"
  abort "registry attestation is not pushed" unless attest.fetch("push-to-registry") == true

  print find_step.call("Derive release metadata").fetch("run")
  File.write(ARGV.fetch(1), find_step.call("Reject existing immutable tag").fetch("run"))
' "$WORKFLOW" "$GUARD_SCRIPT" >"$METADATA_SCRIPT"

if grep -E '^[[:space:]]*uses:' "$WORKFLOW" |
	grep -Ev 'uses: [^@[:space:]]+@[0-9a-f]{40}[[:space:]]+# v[0-9]'; then
	fail "every action must be SHA-pinned with a version comment"
fi

cat >"$FAKE_BIN/go" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "${*: -1}" in
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
			"${repository}:2.45.0-sops.3" \
			"${repository}:2.45.0-sops" \
			"${repository}:lts-sops"
		;;
	go-version) echo 1.26.6 ;;
	base-image) echo docker.io/portainer/compose-unpacker@sha256:base ;;
	base-digest) echo sha256:base ;;
	portainer-version) echo 2.45.0 ;;
	sops-version) echo v3.13.3 ;;
	overlay-revision) echo 3 ;;
	*) echo "unexpected go arguments: $*" >&2; exit 64 ;;
esac
EOF

cat >"$FAKE_BIN/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
[[ "$*" == "buildx imagetools inspect ghcr.io/jbruns/compose-unpacker:2.45.0-sops.3" ]] ||
	exit 64
case "${INSPECT_RESULT:-absent}" in
	absent) echo 'manifest unknown: not found' >&2; exit 1 ;;
	exists) exit 0 ;;
	error) echo 'registry connection failed' >&2; exit 42 ;;
	*) exit 64 ;;
esac
EOF
chmod +x "$FAKE_BIN/go" "$FAKE_BIN/docker"

(
	cd "$ROOT"
	GITHUB_OUTPUT="$GITHUB_OUTPUT_FILE" \
		IMAGE_REPOSITORY=ghcr.io/jbruns/compose-unpacker \
		PATH="$FAKE_BIN:$PATH" \
		bash "$METADATA_SCRIPT"
) >"$OUTPUT" 2>&1 || fail "release metadata step failed"

for expected in \
	'immutable=ghcr.io/jbruns/compose-unpacker:2.45.0-sops.3' \
	'go-version=1.26.6' \
	'base-image=docker.io/portainer/compose-unpacker@sha256:base' \
	'base-digest=sha256:base' \
	'portainer-version=2.45.0' \
	'sops-version=v3.13.3' \
	'overlay-revision=3'; do
	grep -Fx "$expected" "$GITHUB_OUTPUT_FILE" >/dev/null ||
		fail "release metadata output is missing: $expected"
done
grep -Fx 'ghcr.io/jbruns/compose-unpacker:lts-sops' "$GITHUB_OUTPUT_FILE" >/dev/null ||
	fail "release tags output is incomplete"

IMMUTABLE_TAG=ghcr.io/jbruns/compose-unpacker:2.45.0-sops.3 \
	PATH="$FAKE_BIN:$PATH" \
	bash "$GUARD_SCRIPT" >"$OUTPUT" 2>&1 ||
	fail "immutable guard rejected an absent tag"

set +e
IMMUTABLE_TAG=ghcr.io/jbruns/compose-unpacker:2.45.0-sops.3 \
	INSPECT_RESULT=exists \
	PATH="$FAKE_BIN:$PATH" \
	bash "$GUARD_SCRIPT" >"$OUTPUT" 2>&1
status=$?
set -e
[[ $status -ne 0 ]] || fail "immutable guard accepted an existing tag"
grep -F 'immutable tag already exists' "$OUTPUT" >/dev/null ||
	fail "immutable guard did not explain the collision"

set +e
IMMUTABLE_TAG=ghcr.io/jbruns/compose-unpacker:2.45.0-sops.3 \
	INSPECT_RESULT=error \
	PATH="$FAKE_BIN:$PATH" \
	bash "$GUARD_SCRIPT" >"$OUTPUT" 2>&1
status=$?
set -e
[[ $status -ne 0 ]] || fail "immutable guard accepted an inconclusive registry error"
