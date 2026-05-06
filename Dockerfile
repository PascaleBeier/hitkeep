ARG GOLANG_VERSION=1.26.2

FROM golang:$GOLANG_VERSION AS builder

RUN mkdir -p /var/lib/hitkeep/data

FROM gcr.io/distroless/cc-debian13:nonroot

ARG TARGETARCH
ARG HITKEEP_VERSION='snapshot'

LABEL org.opencontainers.image.title="HitKeep" \
    org.opencontainers.image.description="Privacy-first analytics for humans and AI agents, self-hosted or in EU/US cloud." \
    org.opencontainers.image.url="https://hitkeep.com" \
    org.opencontainers.image.source="https://github.com/pascalebeier/hitkeep.git" \
    org.opencontainers.image.version="${HITKEEP_VERSION}" \
    org.opencontainers.image.authors="Pascale Beier (@PascaleBeier)" \
    org.opencontainers.image.licenses="MIT"

COPY --from=builder --chown=nonroot:nonroot /var/lib/hitkeep/data /var/lib/hitkeep/data

WORKDIR /app

COPY --chmod=755 hitkeep-linux-${TARGETARCH} /usr/local/bin/hitkeep

ENV HITKEEP_DB_PATH="/var/lib/hitkeep/data/hitkeep.db"
ENV HITKEEP_ARCHIVE_PATH="/var/lib/hitkeep/data/archive"
VOLUME /var/lib/hitkeep/data

HEALTHCHECK --start-period=60s --start-interval=3s --interval=30s --timeout=5s --retries=3 \
  CMD ["hitkeep", "-healthcheck"]

EXPOSE 8080 7946

ENTRYPOINT ["hitkeep"]
