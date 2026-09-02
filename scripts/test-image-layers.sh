#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
	echo "usage: $0 IMAGE" >&2
	exit 2
fi

IMAGE=$1
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
SCRATCH="$ROOT/.tmp/test-image-layers.$$"
LEAKY_IMAGE="compose-unpacker-layer-regression:$$"

cleanup() {
	docker image rm -f "$LEAKY_IMAGE" >/dev/null 2>&1 || true
	rm -rf "$SCRATCH"
}
trap cleanup EXIT

rm -rf "$SCRATCH"
mkdir -p "$SCRATCH"
cp "$ROOT/overlay/sopsdecrypt/testdata/age-key.txt" "$SCRATCH/"
cp "$ROOT/overlay/sopsdecrypt/testdata/config.env.expected" "$SCRATCH/"

cat >"$SCRATCH/Dockerfile" <<'EOF'
ARG RUNTIME_IMAGE=scratch
FROM busybox:1.37.0-musl@sha256:fc6dddc4c44b1bfe37f41cae8e67d1693828e8f42a91862816d7953e2c9d3f23 AS helper
FROM ${RUNTIME_IMAGE}
COPY --from=helper /bin/busybox /busybox
COPY age-key.txt config.env.expected /test-fixtures/
RUN ["/busybox", "rm", "-rf", "/test-fixtures", "/busybox"]
EOF

docker build \
	--platform linux/amd64 \
	--pull=false \
	--build-arg "RUNTIME_IMAGE=$IMAGE" \
	--tag "$LEAKY_IMAGE" \
	"$SCRATCH" >/dev/null

"$ROOT/scripts/test-image.sh" "$IMAGE"

if "$ROOT/scripts/test-image.sh" "$LEAKY_IMAGE" >"$SCRATCH/leaky-output" 2>&1; then
	cat "$SCRATCH/leaky-output" >&2
	echo "layer scanner accepted an image containing deleted test secret material" >&2
	exit 1
fi
if ! grep -F "test secret material found in image layer" "$SCRATCH/leaky-output" >/dev/null; then
	cat "$SCRATCH/leaky-output" >&2
	echo "leaky image failed before the layer scanner detected test material" >&2
	exit 1
fi
