#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
	echo "usage: $0 IMAGE" >&2
	exit 2
fi

IMAGE=$1
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
cd "$ROOT"

manifest_value() {
	make --no-print-directory manifest-value \
		MANIFEST=versions.json \
		"FIELD=$1"
}

portainer_version=$(manifest_value portainer-version)
compose_unpacker_commit=$(manifest_value compose-unpacker-commit)
portainer_server_commit=$(manifest_value portainer-server-commit)
sops_version=$(manifest_value sops-version)
base_digest=$(manifest_value base-digest)
overlay_revision=$(manifest_value overlay-revision)
source_revision=$(git rev-parse HEAD)
build_created=$(git show -s --format=%cI HEAD)

docker run --rm --platform linux/amd64 --entrypoint /app/compose-unpacker "$IMAGE" --help
docker run --rm --platform linux/amd64 --entrypoint /app/sops "$IMAGE" --version |
	grep -F "sops ${sops_version#v}"

test "$(docker image inspect "$IMAGE" --format '{{.Architecture}}')" = "amd64"
test "$(docker image inspect "$IMAGE" --format '{{json .Config.Entrypoint}}')" = '["/app/compose-unpacker"]'
test "$(docker image inspect "$IMAGE" --format '{{index .Config.Labels "io.jbruns.portainer.version"}}')" = "$portainer_version"
test "$(docker image inspect "$IMAGE" --format '{{index .Config.Labels "io.jbruns.portainer.compose-unpacker.commit"}}')" = "$compose_unpacker_commit"
test "$(docker image inspect "$IMAGE" --format '{{index .Config.Labels "io.jbruns.portainer.server.commit"}}')" = "$portainer_server_commit"
test "$(docker image inspect "$IMAGE" --format '{{index .Config.Labels "io.jbruns.sops.version"}}')" = "$sops_version"
test "$(docker image inspect "$IMAGE" --format '{{index .Config.Labels "io.jbruns.overlay.revision"}}')" = "$overlay_revision"
test "$(docker image inspect "$IMAGE" --format '{{index .Config.Labels "org.opencontainers.image.base.digest"}}')" = "$base_digest"
test "$(docker image inspect "$IMAGE" --format '{{index .Config.Labels "org.opencontainers.image.revision"}}')" = "$source_revision"
test "$(docker image inspect "$IMAGE" --format '{{index .Config.Labels "org.opencontainers.image.created"}}')" = "$build_created"

archive=".tmp/test-image-scan.$$.tar"
layer=".tmp/test-image-layer.$$.tar"
entries=".tmp/test-image-entries.$$"
trap 'rm -f "$archive" "$layer" "$entries"' EXIT

age_identity=$(grep -m 1 '^AGE-SECRET-KEY-' overlay/sopsdecrypt/testdata/age-key.txt)
plaintext=$(cat overlay/sopsdecrypt/testdata/config.env.expected)
test -n "$age_identity"
test -n "$plaintext"

docker image save -o "$archive" "$IMAGE"
expected_layers=$(docker image inspect "$IMAGE" --format '{{len .RootFS.Layers}}')
scanned_layers=0

while IFS= read -r blob; do
	tar -xOf "$archive" "$blob" >"$layer"
	if ! tar -tf "$layer" >"$entries" 2>/dev/null; then
		continue
	fi
	scanned_layers=$((scanned_layers + 1))

	if grep -E '(^|/)(age-key\.txt|config\.env\.expected|config\.sops\.env)$' "$entries" >/dev/null; then
		echo "test fixture filename found in image layer $blob" >&2
		echo "test secret material found in image layer" >&2
		exit 1
	fi
	if tar -xOf "$layer" 2>/dev/null |
		grep -aF \
			-e "$age_identity" \
			-e "$plaintext" \
			-e 'TEST ONLY: this identity protects no real secret' \
			-e 'DEMO_VALUE=decrypted-test-value' >/dev/null; then
		echo "test fixture content found in image layer $blob" >&2
		echo "test secret material found in image layer" >&2
		exit 1
	fi
done < <(tar -tf "$archive" | grep -E '^blobs/sha256/[[:xdigit:]]{64}$')

if [[ $scanned_layers -ne $expected_layers ]]; then
	echo "scanned $scanned_layers image layers, expected $expected_layers" >&2
	exit 1
fi
