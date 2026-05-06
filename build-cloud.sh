#!/usr/bin/env bash
set -euo pipefail

#
# Build HitKeep Cloud binary for linux/arm64 inside Docker (Ubuntu 22.04).
# Mirrors the CI pipeline: native ARM64 compilation with billing tags.
#
# Usage:
#   ./build-cloud.sh                  # Build linux/arm64 (default, matches prod)
#   ./build-cloud.sh amd64            # Build linux/amd64
#   ./build-cloud.sh arm64 --deploy   # Build and deploy to both regions
#   ./build-cloud.sh arm64 --deploy --eu-only  # Build and deploy to EU only
#

GOARCH="${1:-arm64}"
shift || true

DEPLOY=false
DEPLOY_ARGS=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --deploy) DEPLOY=true; shift ;;
    *)        DEPLOY_ARGS+=("$1"); shift ;;
  esac
done

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GO_VERSION="$(grep '^go ' "$ROOT_DIR/go.mod" | awk '{print $2}')"
OUTPUT="$ROOT_DIR/hitkeep-cloud-linux-${GOARCH}"
BUILD_TAGS="${BUILD_TAGS:-hashicorpmetrics s3 billing tenancy}"
VERSION="snapshot-$(date -u +%Y%m%dT%H%M%S)"

echo "Building HitKeep Cloud (linux/${GOARCH}) with Go ${GO_VERSION}..."
echo "  Tags: ${BUILD_TAGS}"
echo "  Version: ${VERSION}"
echo ""

# Step 1: Build frontend (mirrors CI: build dashboard → clean public preserving embed.go → copy assets)
echo "── Building frontend ──"
cd "$ROOT_DIR/frontend/dashboard"
npm ci --no-fund --no-audit
npm run build:prod
cd "$ROOT_DIR"
mkdir -p public
find public -mindepth 1 ! -name embed.go -exec rm -rf {} +
cp -r frontend/dashboard/dist/dashboard/browser/* public/
echo ""

# Step 2: Build Go binary in Docker
echo "── Building binary in Docker (ubuntu:22.04, linux/${GOARCH}) ──"

docker buildx build \
  --no-cache \
  --platform "linux/${GOARCH}" \
  --build-arg "GO_VERSION=${GO_VERSION}" \
  --build-arg "BUILD_TAGS=${BUILD_TAGS}" \
  --build-arg "VERSION=${VERSION}" \
  --output "type=local,dest=$ROOT_DIR/.build-output" \
  -f - "$ROOT_DIR" <<'DOCKERFILE'
ARG GO_VERSION=1.26.2

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS builder

ARG TARGETOS
ARG TARGETARCH
ARG BUILD_TAGS
ARG VERSION

RUN apt-get update && apt-get install -y --no-install-recommends gcc g++ && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=1 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    CC=gcc CXX=g++ \
    go build \
      -tags "${BUILD_TAGS}" \
      -ldflags="-w -s -X 'hitkeep/cmd.Version=${VERSION}'" \
      -o /out/hitkeep \
      ./cmd/hitkeep/main.go

FROM scratch
COPY --from=builder /out/hitkeep /hitkeep
DOCKERFILE

mv "$ROOT_DIR/.build-output/hitkeep" "$OUTPUT"
rm -rf "$ROOT_DIR/.build-output"
chmod +x "$OUTPUT"

echo ""
echo "Built: $OUTPUT"
ls -lh "$OUTPUT"

# Step 3: Optional deploy
if [[ "$DEPLOY" == "true" ]]; then
  INFRA_DIR="$(cd "$ROOT_DIR/../hitkeep-infra" && pwd)"
  if [[ ! -f "$INFRA_DIR/deploy.sh" ]]; then
    echo "Cannot find $INFRA_DIR/deploy.sh" >&2
    exit 1
  fi
  echo ""
  echo "── Deploying ──"
  exec "$INFRA_DIR/deploy.sh" -f "$OUTPUT" ${DEPLOY_ARGS[@]+"${DEPLOY_ARGS[@]}"}
fi
