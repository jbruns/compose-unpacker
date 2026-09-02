#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
BASE_MANIFEST=

cleanup() {
	if [[ -n "$BASE_MANIFEST" ]]; then
		rm -f "$ROOT/$BASE_MANIFEST"
	fi
}
trap cleanup EXIT

usage() {
	echo "usage: $0 [--image IMAGE | --existing-image IMAGE]" >&2
	exit 2
}

heading() {
	printf '\n==> %s\n' "$1" >&2
}

run_stage() {
	local name=$1
	shift
	heading "$name"
	"$@"
}

manifest_value() {
	local manifest=$1
	local field=$2
	heading "go run ./cmd/manifest-value -manifest $manifest $field"
	make --no-print-directory manifest-value \
		"MANIFEST=$manifest" \
		"FIELD=$field"
}

is_release_impacting() {
	case "$1" in
		versions.json | \
			overlay/* | \
			patches/* | \
			Dockerfile | \
			Makefile | \
			scripts/* | \
			cmd/fetch-sops/* | \
			cmd/manifest-value/* | \
			cmd/prepare/* | \
			internal/fetch/* | \
			internal/manifest/* | \
			internal/prepare/*)
			return 0
			;;
		*)
			return 1
			;;
	esac
}

enforce_release_revision() {
	local changed_paths
	local path
	local release_impacting=false

	heading 'git diff --name-only "$BASE_REF"...HEAD'
	changed_paths=$(git diff --name-only "$BASE_REF"...HEAD)
	while IFS= read -r path; do
		if [[ -n "$path" ]] && is_release_impacting "$path"; then
			release_impacting=true
			break
		fi
	done <<<"$changed_paths"

	if [[ "$release_impacting" != true ]]; then
		return
	fi

	local current_revision
	current_revision=$(manifest_value versions.json overlay-revision)
	heading 'git cat-file -e "$BASE_REF:versions.json"'
	if ! git cat-file -e "$BASE_REF:versions.json" 2>/dev/null; then
		if [[ "$current_revision" -ne 1 ]]; then
			echo "release-impacting changes against a base without versions.json require overlayRevision 1 (found $current_revision)" >&2
			return 1
		fi
		return
	fi

	mkdir -p .tmp
	BASE_MANIFEST=".tmp/validate-base-manifest.$$"
	heading 'git show "$BASE_REF:versions.json"'
	git show "$BASE_REF:versions.json" >"$BASE_MANIFEST"

	local base_portainer
	local base_revision
	local current_portainer
	base_portainer=$(manifest_value "$BASE_MANIFEST" portainer-version)
	base_revision=$(manifest_value "$BASE_MANIFEST" overlay-revision)
	current_portainer=$(manifest_value versions.json portainer-version)

	if [[ "$current_portainer" != "$base_portainer" ]]; then
		if [[ "$current_revision" -ne 1 ]]; then
			echo "release-impacting changes with a new Portainer version require overlayRevision 1 (found $current_revision)" >&2
			return 1
		fi
	elif [[ "$current_revision" -le "$base_revision" ]]; then
		echo "release-impacting changes with unchanged Portainer version require overlayRevision greater than $base_revision (found $current_revision)" >&2
		return 1
	fi
}

check_shell_syntax() {
	find scripts .github/tests -type f -name '*.sh' -print0 |
		xargs -0 -n1 bash -n
}

run_workflow_tests() {
	local test

	for test in .github/tests/*.sh; do
		bash "$test"
	done
}

IMAGE=
BUILD_IMAGE=false
case $# in
	0) ;;
	2)
		[[ -n "$2" ]] || usage
		case "$1" in
			--image) BUILD_IMAGE=true ;;
			--existing-image) ;;
			*) usage ;;
		esac
		IMAGE=$2
		;;
	*)
		usage
		;;
esac

cd "$ROOT"

if [[ -n "${BASE_REF:-}" ]]; then
	enforce_release_revision
fi

run_stage "go test -race ./cmd/... ./internal/..." \
	make --no-print-directory validate-internal-test
run_stage "go vet ./internal/..." \
	make --no-print-directory validate-internal-vet
run_stage 'test -z "$(gofmt -l cmd internal overlay)"' \
	make --no-print-directory validate-format
run_stage "find scripts .github/tests -type f -name '*.sh' -print0 | xargs -0 -n1 bash -n" \
	check_shell_syntax
run_stage 'for test in .github/tests/*.sh; do bash "$test"; done' \
	run_workflow_tests
run_stage "go run ./cmd/prepare" \
	make --no-print-directory prepare
run_stage "(cd .work/upstream/compose-unpacker && go test -race ./...)" \
	make --no-print-directory validate-upstream-test
run_stage "(cd .work/upstream/compose-unpacker && go vet ./...)" \
	make --no-print-directory validate-upstream-vet
run_stage "go run ./cmd/fetch-sops -output .work/dist/sops" \
	make --no-print-directory validate-fetch-sops
run_stage 'SOPS_BINARY="$PWD/.work/dist/sops" go test -tags=integration ./sopsdecrypt' \
	make --no-print-directory validate-integration

LINT_VERSION=$(manifest_value versions.json lint-version)
run_stage 'GOBIN="$PWD/.work/bin" go install golangci-lint@"$LINT_VERSION"' \
	make --no-print-directory validate-install-lint "LINT_VERSION=$LINT_VERSION"
run_stage "golangci-lint run --timeout=10m -c .golangci.yaml ./..." \
	make --no-print-directory validate-lint

if [[ -n "$IMAGE" ]]; then
	if [[ "$BUILD_IMAGE" == true ]]; then
		run_stage "make image IMAGE=$IMAGE" \
			make --no-print-directory image "IMAGE=$IMAGE"
	fi
	run_stage "make test-image IMAGE=$IMAGE" \
		make --no-print-directory test-image "IMAGE=$IMAGE"
fi
