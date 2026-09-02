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
case "\$*" in
	*cmd/manifest-value*)
		echo called >"$SCRATCH/accessor-called"
		printf '%s\n' \
			1.26.6 \
			base@sha256:digest \
			sha256:digest \
			2.45.0 \
			v3.13.3
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
exit 0
EOF

chmod +x "$SCRATCH/fake-go-run" "$SCRATCH/bin/docker"

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
