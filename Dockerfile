# syntax=docker/dockerfile:1

ARG GOLANG_VERSION=1.26.5
ARG NODE_VERSION=24

FROM node:${NODE_VERSION}-bookworm AS frontend-builder

WORKDIR /workspace/frontend/dashboard

COPY frontend/dashboard/package.json frontend/dashboard/package-lock.json frontend/dashboard/.npmrc ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci --no-fund --no-audit

COPY frontend/dashboard ./
RUN mkdir -p /workspace/public && npm run build:prod

FROM golang:${GOLANG_VERSION} AS source-builder

ARG GO_BUILD_TAGS="hashicorpmetrics timetzdata"
ARG HITKEEP_VERSION="snapshot"

WORKDIR /workspace

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY skills ./skills
COPY public/embed.go ./public/embed.go
COPY --from=frontend-builder /workspace/public/ ./public/

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 go build \
      -tags "${GO_BUILD_TAGS}" \
      -ldflags="-w -s -X 'hitkeep/cmd.Version=${HITKEEP_VERSION}'" \
      -o /out/hitkeep \
      ./cmd/hitkeep/main.go

FROM golang:${GOLANG_VERSION} AS data-dir

RUN mkdir -p /var/lib/hitkeep/data

FROM gcr.io/distroless/cc-debian13:nonroot AS runtime

ARG HITKEEP_VERSION='snapshot'
ARG HITKEEP_VARIANT='self-hosted'

LABEL org.opencontainers.image.title="HitKeep" \
    org.opencontainers.image.description="Privacy-first analytics for humans and AI agents, self-hosted or in EU/US cloud." \
    org.opencontainers.image.url="https://hitkeep.com" \
    org.opencontainers.image.source="https://github.com/pascalebeier/hitkeep.git" \
    org.opencontainers.image.version="${HITKEEP_VERSION}" \
    org.opencontainers.image.authors="Pascale Beier (@PascaleBeier)" \
    org.opencontainers.image.licenses="MIT" \
    io.hitkeep.variant="${HITKEEP_VARIANT}"

COPY --from=data-dir --chown=nonroot:nonroot /var/lib/hitkeep/data /var/lib/hitkeep/data

WORKDIR /app

ENV HITKEEP_DB_PATH="/var/lib/hitkeep/data/hitkeep.db"
ENV HITKEEP_ARCHIVE_PATH="/var/lib/hitkeep/data/archive"
VOLUME /var/lib/hitkeep/data

HEALTHCHECK --start-period=60s --start-interval=3s --interval=30s --timeout=5s --retries=3 \
  CMD ["hitkeep", "-healthcheck"]

EXPOSE 8080 7946

ENTRYPOINT ["hitkeep"]

# Local builds compile from source inside BuildKit, so they work from macOS,
# Linux, ARM64, and AMD64 without a host Go/CGo toolchain.
FROM runtime AS local-image

COPY --from=source-builder --chmod=755 /out/hitkeep /usr/local/bin/hitkeep

# CI remains the default target and packages its already-built, audited public
# binaries. Cloud binaries are never copied into this release image.
FROM runtime AS release

ARG TARGETARCH

COPY --chmod=755 hitkeep-linux-${TARGETARCH} /usr/local/bin/hitkeep
