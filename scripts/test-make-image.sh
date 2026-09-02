#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
SCRATCH="$ROOT/.tmp/test-make-image.$$"

cleanup() {
	rm -rf "$SCRATCH"
}
trap cleanup EXIT

rm -rf "$SCRATCH"
mkdir -p "$SCRATCH/bin"

cat >"$SCRATCH/fake-go-run" <<EOF
#!/bin/sh
echo called >"$SCRATCH/go-called"
case "\$*" in
	*cmd/manifest-value*)
		echo called >"$SCRATCH/accessor-called"
		if [ "\${FAKE_ACCESSOR_MODE:-late-failure}" = success ]; then
			printf '%s\n' \
				1.26.6 \
				base@sha256:digest \
				sha256:digest \
				2.45.0 \
				v3.13.3 \
				1 \
				23c8e42176c521cb6745b3ea95233d3a68bbe031 \
				d79ba726cd54395a54cca5e9180609ce52fa7a4f
			exit 0
		fi
		printf '%s\n' \
			1.26.6 \
			base@sha256:digest \
			sha256:digest \
			2.45.0 \
			v3.13.3 \
			1 \
			23c8e42176c521cb6745b3ea95233d3a68bbe031
		exit 1
		;;
	*)
		exit 0
		;;
esac
EOF

cat >"$SCRATCH/bin/docker" <<EOF
#!/bin/sh
echo called >"$SCRATCH/docker-build-called"
printf '%s\n' "\$*" >"$SCRATCH/docker-build-args"
exit 0
EOF

cat >"$SCRATCH/bin/git" <<'EOF'
#!/bin/sh
case "$*" in
	"rev-parse HEAD")
		echo aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
		;;
	"show -s --format=%cI HEAD")
		echo 2026-09-02T23:41:46+00:00
		;;
	*)
		echo "unexpected git arguments: $*" >&2
		exit 64
		;;
esac
EOF

chmod +x "$SCRATCH/fake-go-run" "$SCRATCH/bin/docker" "$SCRATCH/bin/git"

set +e
PATH="$SCRATCH/bin:$PATH" make --no-print-directory -C "$ROOT" image \
	"GO_CACHE_ROOT=$SCRATCH/cache" \
	"GO_RUN=$SCRATCH/fake-go-run" \
	IMAGE=test.invalid/compose-unpacker:test >"$SCRATCH/output" 2>&1
status=$?
set -e

if [[ ! -e "$SCRATCH/accessor-called" ]]; then
	cat "$SCRATCH/output" >&2
	echo "manifest accessor regression did not run" >&2
	exit 1
fi
if [[ $status -eq 0 ]]; then
	cat "$SCRATCH/output" >&2
	echo "image target succeeded after a late manifest accessor failure" >&2
	exit 1
fi
if [[ -e "$SCRATCH/docker-build-called" ]]; then
	cat "$SCRATCH/output" >&2
	echo "docker build ran after a late manifest accessor failure" >&2
	exit 1
fi

rm -f "$SCRATCH/go-called" "$SCRATCH/docker-build-called"
set +e
PATH="$SCRATCH/bin:$PATH" make --no-print-directory -C "$ROOT" image \
	"GO_CACHE_ROOT=$SCRATCH/cache" \
	"GO_RUN=$SCRATCH/fake-go-run" \
	GO_BOOTSTRAP_VERSION=9.9.9 \
	IMAGE=test.invalid/compose-unpacker:test >"$SCRATCH/output" 2>&1
status=$?
set -e

if [[ $status -eq 0 ]]; then
	cat "$SCRATCH/output" >&2
	echo "image target accepted a bootstrap Go version that differs from versions.json" >&2
	exit 1
fi
if [[ -e "$SCRATCH/go-called" ]]; then
	cat "$SCRATCH/output" >&2
	echo "Go work ran before the bootstrap version mismatch was rejected" >&2
	exit 1
fi

rm -f "$SCRATCH/go-called" "$SCRATCH/docker-build-called" "$SCRATCH/docker-build-args"
set +e
FAKE_ACCESSOR_MODE=success \
	PATH="$SCRATCH/bin:$PATH" \
	make --no-print-directory -C "$ROOT" image \
	"GO_CACHE_ROOT=$SCRATCH/cache" \
	"GO_RUN=$SCRATCH/fake-go-run" \
	IMAGE=test.invalid/compose-unpacker:test >"$SCRATCH/output" 2>&1
status=$?
set -e

if [[ $status -ne 0 ]]; then
	cat "$SCRATCH/output" >&2
	echo "image target rejected complete manifest provenance" >&2
	exit 1
fi
for argument in \
	'--build-arg COMPOSE_UNPACKER_COMMIT=23c8e42176c521cb6745b3ea95233d3a68bbe031' \
	'--build-arg PORTAINER_SERVER_COMMIT=d79ba726cd54395a54cca5e9180609ce52fa7a4f' \
	'--build-arg SOURCE_REVISION=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' \
	'--build-arg BUILD_CREATED=2026-09-02T23:41:46+00:00'; do
	grep -F -- "$argument" "$SCRATCH/docker-build-args" >/dev/null || {
		cat "$SCRATCH/docker-build-args" >&2
		echo "docker build is missing provenance argument: $argument" >&2
		exit 1
	}
done
