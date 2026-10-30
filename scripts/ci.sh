#!/bin/sh
# Copyright (C) 2026 group24 AG
# SPDX-License-Identifier: AGPL-3.0-only
#
# The CI of itsm. Runs in the tool container until a remote exists (Hauptplan E48, E52):
#   docker compose run --rm tools scripts/ci.sh
# Every step must pass; the first failure stops the run.
set -eu
cd "$(dirname "$0")/.."

step() { printf '\n== %s\n' "$1"; }

step "license headers"
scripts/check-license.sh

step "go: format"
unformatted=$(gofmt -l cmd internal db)
if [ -n "$unformatted" ]; then
  echo "not gofmt-formatted:"; echo "$unformatted"; exit 1
fi

step "go: sqlc code is up to date"
sqlc diff

step "go: generated API server is up to date (api/openapi.yaml, oapi-codegen)"
cp internal/httpapi/gen/api.gen.go /tmp/api.gen.go.before
go tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml
if ! cmp -s /tmp/api.gen.go.before internal/httpapi/gen/api.gen.go; then
  echo "internal/httpapi/gen/api.gen.go was out of date; regenerated, please commit it"; exit 1
fi

step "go: vet"
go vet ./...

step "go: lint"
golangci-lint run ./...

step "go: only records and query touch type tables (E34)"
sh scripts/check-type-tables.sh

step "go: test"
go test -race -count=1 ./...

step "go: build"
go build -o /tmp/itsm ./cmd/itsm

step "load: quick stage, A/B against the baseline build (Hauptplan E32, E63, E69)"
export ITSM_DATABASE_URL="$ITSM_TEST_DATABASE_URL" ITSM_VALKEY_ADDR="$ITSM_TEST_VALKEY_ADDR"
export ITSM_HTTP_ADDR=127.0.0.1:18080 ITSM_LOG_LEVEL=warn
# LZ5 sends NDJSON streams, which the asynchronous bulk keeps in the file store (PL32).
export ITSM_FILES_DIR=/tmp/itsm-load-files
mkdir -p "$ITSM_FILES_DIR"
# Its jobs run on a queue of their own: an instance on the same database (the local container)
# knows bulk.apply too and would take them, without the stream in its file store (WF3).
export ITSM_BULK_QUEUE=bulkload
/tmp/itsm migrate >/dev/null
# The load scenarios LZ1, LZ2 and LZ4 work on the probe package (Hauptplan H13).
/tmp/itsm package install packages/core >/dev/null 2>&1
/tmp/itsm package install packages/probe >/dev/null 2>&1
# The load tool signs in with this account (test/load/internal/scenario); the password is the
# tool's default for the test database only.
# The password must not contain the username (PL28); an account from before gets it set.
printf 'Messlauf-Pw7!' | /tmp/itsm user create load --display-name "Lasttest" >/dev/null 2>&1 ||
  printf 'Messlauf-Pw7!' | /tmp/itsm user passwd load >/dev/null 2>&1 || true
/tmp/itsm user grant load admin >/dev/null 2>&1 || true
# The browser test of UI8 needs a second person: a change by one's own account is not shown as
# live, and one does not see oneself on a record.
printf 'Zweite-Sicht8!' | /tmp/itsm user create kollegin --display-name "Kollegin" >/dev/null 2>&1 ||
  printf 'Zweite-Sicht8!' | /tmp/itsm user passwd kollegin >/dev/null 2>&1 || true
/tmp/itsm user grant kollegin admin >/dev/null 2>&1 || true
# The browser test of UI9 switches its account to English; an account of its own keeps the
# other tests, which run at the same time, in German.
printf 'Eigene-Wahl9!' | /tmp/itsm user create einsteller --display-name "Einsteller" >/dev/null 2>&1 ||
  printf 'Eigene-Wahl9!' | /tmp/itsm user passwd einsteller >/dev/null 2>&1 || true
/tmp/itsm user grant einsteller admin >/dev/null 2>&1 || true
/tmp/itsm serve --role=all &
server=$!
trap 'kill "$server" "${baseline:-}" 2>/dev/null || true' EXIT
tries=0
until /tmp/itsm healthcheck 2>/dev/null; do
  tries=$((tries + 1))
  if [ "$tries" -gt 60 ]; then echo "itsm did not start"; exit 1; fi
  sleep 0.5
done
ab=""
BASELINE_BIN=/var/lib/itsm-load/itsm-baseline
if [ -x "$BASELINE_BIN" ]; then
  ITSM_HTTP_ADDR=127.0.0.1:18081 "$BASELINE_BIN" serve --role=all &
  baseline=$!
  tries=0
  until ITSM_HTTP_ADDR=127.0.0.1:18081 "$BASELINE_BIN" healthcheck 2>/dev/null; do
    tries=$((tries + 1))
    if [ "$tries" -gt 60 ]; then echo "baseline itsm did not start"; exit 1; fi
    sleep 0.5
  done
  ab="--baseline-target=http://127.0.0.1:18081 --baseline-sha256=$(sha256sum "$BASELINE_BIN" | cut -d' ' -f1)"
else
  echo "no baseline build below /var/lib/itsm-load: measuring the new build only"
fi
# shellcheck disable=SC2086
go run ./test/load run --stage=quick --target=http://127.0.0.1:18080 $ab --wait-for-idle=120s --out=/tmp/load-quick.json
go run ./test/load gate --stage=quick --result=/tmp/load-quick.json --baseline=test/load/baseline.json
kill "$server" "${baseline:-}" 2>/dev/null || true
trap - EXIT

step "web: install"
cd web
pnpm install --frozen-lockfile

# What itsm ships (the Go modules of the binary, the web packages of web/dist) has only allowed
# licenses (PL29, Hauptplan E15); the list and the exceptions are test/licenses/licenses.txt.
step "licenses: Go modules and web packages that are shipped (Hauptplan E15)"
pnpm licenses list --prod --json >/tmp/web-licenses.json
(cd .. && go run ./test/licenses --web=/tmp/web-licenses.json)

step "web: generated API types are up to date (api/openapi.yaml, openapi-typescript)"
cp src/api/schema.d.ts /tmp/schema.d.ts
pnpm api:gen >/dev/null
cmp /tmp/schema.d.ts src/api/schema.d.ts

step "web: lint"
pnpm lint

step "web: format"
pnpm format:check

step "web: test"
pnpm test

step "web: build"
pnpm build

step "web: browser tests (Playwright with axe, UI1)"
cd ..
# All roles: the live test (UI8) needs the dispatcher of this server, not one that happens to
# run on the same database.
ITSM_WEB_DIR=web/dist ITSM_HTTP_ADDR=127.0.0.1:18082 /tmp/itsm serve --role=all >/tmp/itsm-e2e.log 2>&1 &
e2e_server=$!
trap 'kill "$e2e_server" 2>/dev/null || true' EXIT
for _ in $(seq 1 50); do
  curl -sf http://127.0.0.1:18082/healthz >/dev/null 2>&1 && break
  sleep 0.2
done
# The browser test of UI10 needs a user of the portal: her own records of probe, creating and
# answering, no workplace. The command line only assigns roles, so the role comes over the API
# of this server, signed in as the administrator load.
jar=$(mktemp)
curl -sf -c "$jar" -H 'Content-Type: application/json' -H 'X-Requested-With: itsm' \
  -d '{"username": "load", "password": "Messlauf-Pw7!"}' http://127.0.0.1:18082/api/v1/auth/login >/dev/null
as_admin() { curl -sf -b "$jar" -H 'Content-Type: application/json' -H 'X-Requested-With: itsm' "$@" >/dev/null 2>&1 || true; }
as_admin -X POST -d '{"name": "e2e_portal", "label": {"de": "Portal (Test)"}}' http://127.0.0.1:18082/api/v1/roles
for permission in \
  '{"type_name": "probe.probe", "action": "read", "scope": "own"}' \
  '{"type_name": "probe.probe", "action": "read", "scope": "involved"}' \
  '{"type_name": "probe.probe", "action": "create", "scope": "all"}' \
  '{"type_name": "probe.probe", "action": "comment", "scope": "own"}' \
  '{"type_name": "probe.probe", "action": "comment", "scope": "involved"}'; do
  as_admin -X PUT -d "$permission" http://127.0.0.1:18082/api/v1/roles/e2e_portal/permissions
done
rm -f "$jar"
printf 'Nur-Portal10!' | /tmp/itsm user create nutzerin --display-name "Nutzerin" >/dev/null 2>&1 ||
  printf 'Nur-Portal10!' | /tmp/itsm user passwd nutzerin >/dev/null 2>&1 || true
/tmp/itsm user grant nutzerin e2e_portal >/dev/null 2>&1 || true
# The browser test of UI12 fills a form in English: a user of the portal of her own, so that
# nutzerin stays German for the test of UI10 running at the same time.
printf 'Portal-Sprache12!' | /tmp/itsm user create englisch --display-name "Englisch" >/dev/null 2>&1 ||
  printf 'Portal-Sprache12!' | /tmp/itsm user passwd englisch >/dev/null 2>&1 || true
/tmp/itsm user grant englisch e2e_portal >/dev/null 2>&1 || true
cd web
ITSM_E2E_URL=http://127.0.0.1:18082 pnpm e2e
cd ..
kill "$e2e_server" 2>/dev/null || true
trap - EXIT

printf '\nci: all steps passed\n'
