# check=skip=InvalidDefaultArgInFrom

ARG GO_VERSION
ARG BASE_IMAGE
FROM golang:${GO_VERSION}-alpine AS builder
WORKDIR /src
COPY .work/upstream/portainer /src/portainer
COPY .work/upstream/compose-unpacker /src/compose-unpacker
WORKDIR /src/compose-unpacker
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/compose-unpacker .

FROM ${BASE_IMAGE}
ARG PORTAINER_VERSION
ARG SOPS_VERSION
ARG OVERLAY_REVISION
ARG SOURCE_REVISION
ARG BASE_DIGEST
COPY --from=builder /out/compose-unpacker /app/compose-unpacker
COPY .work/dist/sops /app/sops
LABEL org.opencontainers.image.source="https://github.com/jbruns/compose-unpacker" \
      org.opencontainers.image.revision="${SOURCE_REVISION}" \
      org.opencontainers.image.base.name="docker.io/portainer/compose-unpacker" \
      org.opencontainers.image.base.digest="${BASE_DIGEST}" \
      io.jbruns.portainer.version="${PORTAINER_VERSION}" \
      io.jbruns.sops.version="${SOPS_VERSION}" \
      io.jbruns.overlay.revision="${OVERLAY_REVISION}"
ENTRYPOINT ["/app/compose-unpacker"]
