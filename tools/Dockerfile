# Copyright (C) 2026 group24 AG
# SPDX-License-Identifier: AGPL-3.0-only
#
# The tool container (Hauptplan E52): everything needed to build, test and lint, so nothing is
# installed on the developer machine. Versions from Hauptplan E53.

FROM node:24.22.0-slim AS node

FROM golang:1.27.3
COPY --from=node /usr/local/bin/node /usr/local/bin/node
COPY --from=node /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -s ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
 && ln -s ../lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx \
 && npm install --global pnpm@12.14.0

# Chromium for the browser tests of the web interface (UI1, Hauptplan E41/E42); the version
# must match @playwright/test in web/package.json. The browsers live outside /root/.cache,
# which compose.yaml covers with the Go build cache volume.
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright
RUN npx --yes playwright@1.64.1 install --with-deps chromium chromium-headless-shell \
 && rm -rf /root/.npm

# pg_dump and pg_restore of the server's major version, for the test of backing up and
# restoring a tenant's database (PL47d, Hauptplan E144); from the repository of PostgreSQL,
# since Debian ships an older client.
RUN apt-get update \
 && apt-get install -y --no-install-recommends postgresql-common \
 && /usr/share/postgresql-common/pgdg/apt.postgresql.org.sh -y \
 && apt-get install -y --no-install-recommends postgresql-client-18 \
 && rm -rf /var/lib/apt/lists/*

RUN go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 \
 && go install github.com/pressly/goose/v3/cmd/goose@v3.29.0 \
 && go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 \
 && rm -rf /root/.cache/go-build

WORKDIR /src
