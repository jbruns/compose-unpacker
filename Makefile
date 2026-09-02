GO_BOOTSTRAP_IMAGE := golang:1.26.6
GO_CACHE_ROOT := .tmp
HOST_UID := $(shell id -u)
HOST_GID := $(shell id -g)
IMAGE ?= ghcr.io/jbruns/compose-unpacker:test

GO_RUN = docker run --rm --platform linux/amd64 \
	--user "$(HOST_UID):$(HOST_GID)" \
	-e PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin \
	-e HOME=/repo/$(GO_CACHE_ROOT)/home \
	-e TMPDIR=/repo/$(GO_CACHE_ROOT)/test-temp \
	-e GOCACHE=/repo/$(GO_CACHE_ROOT)/go-build \
	-e GOMODCACHE=/repo/$(GO_CACHE_ROOT)/go-mod \
	-e GOPATH=/repo/$(GO_CACHE_ROOT)/go-path \
	-e GIT_CONFIG_COUNT=1 \
	-e GIT_CONFIG_KEY_0=safe.directory \
	-e GIT_CONFIG_VALUE_0='*' \
	-v "$(CURDIR):/repo" \
	-w /repo \
	$(GO_BOOTSTRAP_IMAGE)

.PHONY: prepare test test-integration image test-image validate clean go-cache
.NOTPARALLEL:

go-cache:
	@mkdir -p \
		$(GO_CACHE_ROOT)/home \
		$(GO_CACHE_ROOT)/test-temp \
		$(GO_CACHE_ROOT)/go-build \
		$(GO_CACHE_ROOT)/go-mod \
		$(GO_CACHE_ROOT)/go-path

prepare: go-cache
	$(GO_RUN) /usr/local/go/bin/go run ./cmd/prepare

test: prepare
	$(GO_RUN) /bin/sh -lc '\
		/usr/local/go/bin/go test ./cmd/... ./internal/... ./overlay/sopsdecrypt -count=1 && \
		cd /repo/.work/upstream/compose-unpacker && \
		/usr/local/go/bin/go test ./... -count=1'

test-integration: prepare
	$(GO_RUN) /usr/local/go/bin/go run ./cmd/fetch-sops -output .work/dist/sops
	$(GO_RUN) /bin/sh -lc '\
		cd /repo/.work/upstream/compose-unpacker && \
		SOPS_BINARY=/repo/.work/dist/sops \
		/usr/local/go/bin/go test -tags=integration ./sopsdecrypt -count=1'

image: prepare
	$(GO_RUN) /usr/local/go/bin/go run ./cmd/fetch-sops -output .work/dist/sops
	@set -- $$($(GO_RUN) /bin/sh -lc '\
		for field in go-version base-image base-digest portainer-version sops-version overlay-revision; do \
			/usr/local/go/bin/go run ./cmd/manifest-value "$$field" || exit; \
		done'); \
	test "$$#" -eq 6; \
	docker buildx build \
		--platform linux/amd64 \
		--load \
		--build-arg GO_VERSION="$$1" \
		--build-arg BASE_IMAGE="$$2" \
		--build-arg BASE_DIGEST="$$3" \
		--build-arg PORTAINER_VERSION="$$4" \
		--build-arg SOPS_VERSION="$$5" \
		--build-arg OVERLAY_REVISION="$$6" \
		--build-arg SOURCE_REVISION="$$(git rev-parse HEAD)" \
		--tag "$(IMAGE)" \
		.

test-image:
	./scripts/test-image.sh "$(IMAGE)"

validate: test test-integration image test-image

clean:
	rm -rf .work coverage.out
