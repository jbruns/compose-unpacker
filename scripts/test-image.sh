#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
	echo "usage: $0 IMAGE" >&2
	exit 2
fi

IMAGE=$1
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
cd "$ROOT"

mkdir -p .tmp/{home,test-temp,go-build,go-mod,go-path}
go_container=(
	docker run --rm --platform linux/amd64
	--user "$(id -u):$(id -g)"
	-e PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin
	-e HOME=/repo/.tmp/home
	-e TMPDIR=/repo/.tmp/test-temp
	-e GOCACHE=/repo/.tmp/go-build
	-e GOMODCACHE=/repo/.tmp/go-mod
	-e GOPATH=/repo/.tmp/go-path
	-e GIT_CONFIG_COUNT=1
	-e GIT_CONFIG_KEY_0=safe.directory
	-e 'GIT_CONFIG_VALUE_0=*'
	-v "$ROOT:/repo"
	-w /repo
	golang:1.26.6
)

manifest_value() {
	"${go_container[@]}" /usr/local/go/bin/go run ./cmd/manifest-value "$1"
}

portainer_version=$(manifest_value portainer-version)
sops_version=$(manifest_value sops-version)
base_digest=$(manifest_value base-digest)
overlay_revision=$(manifest_value overlay-revision)

docker run --rm --platform linux/amd64 --entrypoint /app/compose-unpacker "$IMAGE" --help
docker run --rm --platform linux/amd64 --entrypoint /app/sops "$IMAGE" --version |
	grep -F "sops ${sops_version#v}"

test "$(docker image inspect "$IMAGE" --format '{{.Architecture}}')" = "amd64"
test "$(docker image inspect "$IMAGE" --format '{{json .Config.Entrypoint}}')" = '["/app/compose-unpacker"]'
test "$(docker image inspect "$IMAGE" --format '{{index .Config.Labels "io.jbruns.portainer.version"}}')" = "$portainer_version"
test "$(docker image inspect "$IMAGE" --format '{{index .Config.Labels "io.jbruns.sops.version"}}')" = "$sops_version"
test "$(docker image inspect "$IMAGE" --format '{{index .Config.Labels "io.jbruns.overlay.revision"}}')" = "$overlay_revision"
test "$(docker image inspect "$IMAGE" --format '{{index .Config.Labels "org.opencontainers.image.base.digest"}}')" = "$base_digest"

container=$(docker create --platform linux/amd64 "$IMAGE")
trap 'docker rm -f "$container" >/dev/null' EXIT
if docker export "$container" | tar -tf - | grep -E '(^|/)(age-key\.txt|config\.env\.expected|config\.sops\.env)$'; then
	echo "test fixtures entered the final image filesystem" >&2
	exit 1
fi
if docker export "$container" | tar -xOf - 2>/dev/null |
	grep -aE 'TEST ONLY: this identity protects no real secret|DEMO_VALUE=decrypted-test-value'; then
	echo "test fixture content entered the final image filesystem" >&2
	exit 1
fi
