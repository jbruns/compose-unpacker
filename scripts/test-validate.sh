#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
SCRATCH="$ROOT/.tmp/test-validate.$$"
FIXTURE="$SCRATCH/repository"
FAKE_BIN="$FIXTURE/.test-bin"
OUTPUT="$SCRATCH/output"
MAKE_LOG="$SCRATCH/make.log"

cleanup() {
	rm -rf "$SCRATCH"
}
trap cleanup EXIT

fail() {
	cat "$OUTPUT" >&2 2>/dev/null || true
	echo "$*" >&2
	exit 1
}

if [[ ! -x "$ROOT/scripts/validate.sh" ]]; then
	echo "scripts/validate.sh is missing or not executable" >&2
	exit 1
fi

rm -rf "$SCRATCH"
mkdir -p "$FIXTURE/scripts" "$FIXTURE/.github/tests" "$FAKE_BIN"
cp "$ROOT/scripts/validate.sh" "$FIXTURE/scripts/validate.sh"

cat >"$FAKE_BIN/make" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

target=
manifest=versions.json
field=
for argument in "$@"; do
	case "$argument" in
		--*) ;;
		MANIFEST=*) manifest=${argument#MANIFEST=} ;;
		FIELD=*) field=${argument#FIELD=} ;;
		*=*) ;;
		*) [[ -n "$target" ]] || target=$argument ;;
	esac
done

if [[ "$target" == manifest-value ]]; then
	printf 'manifest-value:%s:%s\n' "$manifest" "$field" >>"$MAKE_LOG"
	case "$field" in
		portainer-version)
			sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$manifest" |
				head -n 1
			;;
		overlay-revision)
			sed -n 's/.*"overlayRevision"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p' "$manifest"
			;;
		lint-version)
			echo v2.13.2
			;;
		*)
			echo "unexpected manifest field: $field" >&2
			exit 2
			;;
	esac
	exit
fi

printf '%s\n' "$target" >>"$MAKE_LOG"
if [[ "$target" == "${FAIL_TARGET:-}" ]]; then
	exit 23
fi
EOF
chmod +x "$FAKE_BIN/make" "$FIXTURE/scripts/validate.sh"

cat >"$FIXTURE/.github/tests/test-release-workflow.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' workflow-test:release >>"$MAKE_LOG"
EOF
cat >"$FIXTURE/.github/tests/test-update-workflow.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' workflow-test:update >>"$MAKE_LOG"
EOF
chmod +x "$FIXTURE/.github/tests/"*.sh

write_manifest() {
	local version=$1
	local revision=$2
	printf '{"portainer":{"version":"%s"},"overlayRevision":%s}\n' \
		"$version" "$revision" >"$FIXTURE/versions.json"
}

run_validation() {
	local base_ref=$1
	local fail_target=$2
	shift 2
	: >"$MAKE_LOG"
	set +e
	(
		cd "$FIXTURE"
		BASE_REF="$base_ref" \
			FAIL_TARGET="$fail_target" \
			MAKE_LOG="$MAKE_LOG" \
			PATH="$FAKE_BIN:$PATH" \
			./scripts/validate.sh "$@"
	) >"$OUTPUT" 2>&1
	RUN_STATUS=$?
	set -e
}

printf '.tmp/\n' >"$FIXTURE/.gitignore"
git -C "$FIXTURE" init -q
git -C "$FIXTURE" config user.name "Validation Test"
git -C "$FIXTURE" config user.email "validation-test@example.invalid"
git -C "$FIXTURE" add .
git -C "$FIXTURE" commit -q -m "baseline without manifest"
NO_MANIFEST_BASE=$(git -C "$FIXTURE" rev-parse HEAD)
write_manifest 2.45.0 1
git -C "$FIXTURE" add versions.json
git -C "$FIXTURE" commit -q -m "add initial manifest"
BASE_COMMIT=$(git -C "$FIXTURE" rev-parse HEAD)

run_validation "" "" --image example.invalid/compose-unpacker:test
[[ $RUN_STATUS -eq 0 ]] || fail "complete validation command sequence failed"
cat >"$SCRATCH/expected-make-log" <<'EOF'
validate-internal-test
validate-internal-vet
validate-format
workflow-test:release
workflow-test:update
prepare
validate-repository-assets
validate-upstream-test
validate-upstream-vet
validate-fetch-sops
validate-integration
manifest-value:versions.json:lint-version
validate-install-lint
validate-lint
image
test-image
EOF
if ! diff -u "$SCRATCH/expected-make-log" "$MAKE_LOG" >"$SCRATCH/log-diff"; then
	cat "$SCRATCH/log-diff" >&2
	fail "validation commands did not run in the required order"
fi

for heading in \
	"go test -race ./cmd/... ./internal/..." \
	"go vet ./internal/..." \
	'test -z "$(gofmt -l cmd internal overlay)"' \
	"find scripts .github/tests -type f -name '*.sh' -print0 | xargs -0 -n1 bash -n" \
	'for test in .github/tests/*.sh; do bash "$test"; done' \
	"go run ./cmd/prepare" \
	"./scripts/test-repository-assets.sh" \
	"(cd .work/upstream/compose-unpacker && go test -race ./...)" \
	"(cd .work/upstream/compose-unpacker && go vet ./...)" \
	"go run ./cmd/fetch-sops -output .work/dist/sops" \
	'SOPS_BINARY="$PWD/.work/dist/sops" go test -tags=integration ./sopsdecrypt' \
	'GOBIN="$PWD/.work/bin" go install golangci-lint@"$LINT_VERSION"' \
	"golangci-lint run --timeout=10m -c .golangci.yaml ./..." \
	"make image IMAGE=example.invalid/compose-unpacker:test" \
	"make test-image IMAGE=example.invalid/compose-unpacker:test"; do
	grep -F "==> $heading" "$OUTPUT" >/dev/null ||
		fail "missing stage heading: $heading"
done

run_validation "" "" --existing-image example.invalid/compose-unpacker:validated
[[ $RUN_STATUS -eq 0 ]] || fail "validation rejected an existing image"
cat >"$SCRATCH/expected-make-log" <<'EOF'
validate-internal-test
validate-internal-vet
validate-format
workflow-test:release
workflow-test:update
prepare
validate-repository-assets
validate-upstream-test
validate-upstream-vet
validate-fetch-sops
validate-integration
manifest-value:versions.json:lint-version
validate-install-lint
validate-lint
test-image
EOF
if ! diff -u "$SCRATCH/expected-make-log" "$MAKE_LOG" >"$SCRATCH/log-diff"; then
	cat "$SCRATCH/log-diff" >&2
	fail "existing-image validation did not reuse the supplied artifact"
fi
grep -F "==> make test-image IMAGE=example.invalid/compose-unpacker:validated" "$OUTPUT" >/dev/null ||
	fail "existing-image validation did not smoke-test the supplied artifact"
if grep -F "==> make image" "$OUTPUT" >/dev/null; then
	fail "existing-image validation rebuilt the supplied artifact"
fi

run_validation "" "validate-internal-vet"
[[ $RUN_STATUS -eq 23 ]] || fail "validation did not preserve the first failing command status"
printf '%s\n' validate-internal-test validate-internal-vet >"$SCRATCH/expected-make-log"
if ! diff -u "$SCRATCH/expected-make-log" "$MAKE_LOG" >"$SCRATCH/log-diff"; then
	cat "$SCRATCH/log-diff" >&2
	fail "validation continued after the first failure"
fi

run_validation "" "" --image
[[ $RUN_STATUS -eq 2 ]] || fail "--image without a value did not return usage status 2"
run_validation "" "" --unknown
[[ $RUN_STATUS -eq 2 ]] || fail "an unknown argument did not return usage status 2"

cat >"$FIXTURE/.github/tests/test-invalid-syntax.sh" <<'EOF'
#!/usr/bin/env bash
if then
EOF
run_validation "" ""
[[ $RUN_STATUS -ne 0 ]] || fail "invalid workflow-test shell syntax was accepted"
if grep -F '==> for test in .github/tests/*.sh; do bash "$test"; done' "$OUTPUT" >/dev/null; then
	fail "workflow tests started before all owned shell syntax was checked"
fi
rm "$FIXTURE/.github/tests/test-invalid-syntax.sh"

git -C "$FIXTURE" reset -q --hard "$BASE_COMMIT"
run_validation "$NO_MANIFEST_BASE" ""
[[ $RUN_STATUS -eq 0 ]] || fail "initial revision 1 was rejected against a base without versions.json"

write_manifest 2.45.0 2
git -C "$FIXTURE" add versions.json
git -C "$FIXTURE" commit -q -m "use stale initial revision"
run_validation "$NO_MANIFEST_BASE" ""
[[ $RUN_STATUS -ne 0 ]] || fail "base without versions.json accepted an initial revision other than 1"
grep -F "base without versions.json require overlayRevision 1" "$OUTPUT" >/dev/null ||
	fail "missing base manifest produced the wrong revision error"

release_paths=(
	versions.json
	overlay/file.go
	patches/change.patch
	Dockerfile
	Makefile
	scripts/helper.sh
	cmd/fetch-sops/file.go
	cmd/manifest-value/file.go
	cmd/prepare/file.go
	internal/fetch/file.go
	internal/manifest/file.go
	internal/prepare/file.go
)

for path in "${release_paths[@]}"; do
	git -C "$FIXTURE" reset -q --hard "$BASE_COMMIT"
	if [[ "$path" == versions.json ]]; then
		printf '\n' >>"$FIXTURE/$path"
	else
		mkdir -p "$(dirname "$FIXTURE/$path")"
		if [[ "$path" == *.sh ]]; then
			printf '#!/bin/sh\n:\n' >"$FIXTURE/$path"
		else
			echo changed >"$FIXTURE/$path"
		fi
	fi
	git -C "$FIXTURE" add "$path"
	git -C "$FIXTURE" commit -q -m "change $path"

	run_validation "$BASE_COMMIT" ""
	[[ $RUN_STATUS -ne 0 ]] ||
		fail "release-impacting path did not enforce a revision increase: $path"
	grep -F "require overlayRevision greater than 1" "$OUTPUT" >/dev/null ||
		fail "release-impacting path produced the wrong revision error: $path"
done

git -C "$FIXTURE" reset -q --hard "$BASE_COMMIT"
mkdir -p "$FIXTURE/cmd/update-versions" "$FIXTURE/internal/update" "$FIXTURE/docs"
echo changed >"$FIXTURE/cmd/update-versions/file.go"
echo changed >"$FIXTURE/internal/update/file.go"
echo changed >"$FIXTURE/docs/maintenance.md"
git -C "$FIXTURE" add .
git -C "$FIXTURE" commit -q -m "change non-release paths"
run_validation "$BASE_COMMIT" ""
[[ $RUN_STATUS -eq 0 ]] || fail "non-release-impacting paths required a revision increase"

git -C "$FIXTURE" reset -q --hard "$BASE_COMMIT"
write_manifest 2.45.0 2
git -C "$FIXTURE" add versions.json
git -C "$FIXTURE" commit -q -m "increment revision"
run_validation "$BASE_COMMIT" ""
[[ $RUN_STATUS -eq 0 ]] || fail "a strictly larger revision was rejected"

git -C "$FIXTURE" reset -q --hard "$BASE_COMMIT"
write_manifest 2.46.0 2
git -C "$FIXTURE" add versions.json
git -C "$FIXTURE" commit -q -m "change version with stale revision"
run_validation "$BASE_COMMIT" ""
[[ $RUN_STATUS -ne 0 ]] || fail "a new Portainer version accepted a revision other than 1"
grep -F "require overlayRevision 1" "$OUTPUT" >/dev/null ||
	fail "new Portainer version produced the wrong revision error"

git -C "$FIXTURE" reset -q --hard "$BASE_COMMIT"
write_manifest 2.46.0 1
git -C "$FIXTURE" add versions.json
git -C "$FIXTURE" commit -q -m "change version with reset revision"
run_validation "$BASE_COMMIT" ""
[[ $RUN_STATUS -eq 0 ]] || fail "a new Portainer version with revision 1 was rejected"

if find "$FIXTURE/.tmp" -type f -name 'validate-base-manifest.*' -print -quit |
	grep -q .; then
	fail "temporary base manifest was not removed"
fi
