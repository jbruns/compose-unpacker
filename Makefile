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

.PHONY: prepare test test-integration image test-image validate clean go-cache \
	manifest-value validate-internal-test validate-internal-vet validate-format \
	validate-upstream-test validate-upstream-vet validate-fetch-sops \
	validate-integration validate-install-lint validate-lint
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
	./scripts/test-make-image.sh
	./scripts/test-validate.sh

test-integration: prepare
	$(GO_RUN) /usr/local/go/bin/go run ./cmd/fetch-sops -output .work/dist/sops
	$(GO_RUN) /bin/sh -lc '\
		cd /repo/.work/upstream/compose-unpacker && \
		SOPS_BINARY=/repo/.work/dist/sops \
		/usr/local/go/bin/go test -tags=integration ./sopsdecrypt -count=1'

image: prepare
	$(GO_RUN) /usr/local/go/bin/go run ./cmd/fetch-sops -output .work/dist/sops
	@set -eu; \
	manifest_values=$$($(GO_RUN) /bin/sh -lc '\
		for field in go-version base-image base-digest portainer-version sops-version overlay-revision; do \
			/usr/local/go/bin/go run ./cmd/manifest-value "$$field" || exit; \
		done'); \
	set -- $$manifest_values; \
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
	./scripts/test-image-layers.sh "$(IMAGE)"

manifest-value: go-cache
	@$(GO_RUN) /usr/local/go/bin/go run ./cmd/manifest-value \
		-manifest "$(MANIFEST)" "$(FIELD)"

validate-internal-test: go-cache
	$(GO_RUN) /usr/local/go/bin/go test -race ./internal/...

validate-internal-vet: go-cache
	$(GO_RUN) /usr/local/go/bin/go vet ./internal/...

validate-format: go-cache
	$(GO_RUN) /bin/sh -lc \
		'test -z "$$(/usr/local/go/bin/gofmt -l cmd internal overlay)"'

validate-upstream-test: go-cache
	$(GO_RUN) /bin/sh -lc '\
		cd /repo/.work/upstream/compose-unpacker && \
		/usr/local/go/bin/go test -race ./...'

validate-upstream-vet: go-cache
	$(GO_RUN) /bin/sh -lc '\
		cd /repo/.work/upstream/compose-unpacker && \
		/usr/local/go/bin/go vet ./...'

validate-fetch-sops: go-cache
	$(GO_RUN) /usr/local/go/bin/go run ./cmd/fetch-sops \
		-output .work/dist/sops

validate-integration: go-cache
	$(GO_RUN) /bin/sh -lc '\
		cd /repo/.work/upstream/compose-unpacker && \
		SOPS_BINARY=/repo/.work/dist/sops \
		/usr/local/go/bin/go test -tags=integration ./sopsdecrypt'

validate-install-lint: go-cache
	@mkdir -p .work/bin
	$(GO_RUN) /bin/sh -lc '\
		GOBIN=/repo/.work/bin \
		/usr/local/go/bin/go install \
		github.com/golangci/golangci-lint/v2/cmd/golangci-lint@"$(LINT_VERSION)"'

validate-lint: go-cache
	$(GO_RUN) /bin/sh -c '\
		cd /repo/.work/upstream/compose-unpacker && \
		/repo/.work/bin/golangci-lint run --timeout=10m -c .golangci.yaml ./...'

validate:
	./scripts/validate.sh --image "$(IMAGE)"

clean:
	rm -rf .work coverage.out
