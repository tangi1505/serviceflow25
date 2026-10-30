# Copyright (C) 2026 group24 AG
# SPDX-License-Identifier: AGPL-3.0-only
#
# The itsm image: one binary (Hauptplan E35) plus the built web interface (architektur.md §10).
# Versions from Hauptplan E53.

FROM node:24.22.0-slim AS web
RUN npm install --global pnpm@12.14.0
WORKDIR /src/web
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

FROM golang:1.27.3 AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X group24.de/itsm/internal/platform.Version=${VERSION}" \
      -o /out/itsm ./cmd/itsm
# The files directory (PL18, E22), owned by the user the image runs as, so that a named volume
# mounted there starts out writable.
RUN mkdir -p /out/data/files

FROM scratch
COPY --from=go /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=go /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=go /out/itsm /itsm
COPY --from=web /src/web/dist /web
COPY --from=go --chown=65532:65532 /out/data /data
ENV ITSM_WEB_DIR=/web \
    ITSM_HTTP_ADDR=:8080 \
    ITSM_FILES_DIR=/data/files
USER 65532:65532
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD ["/itsm", "healthcheck"]
ENTRYPOINT ["/itsm"]
CMD ["serve"]
