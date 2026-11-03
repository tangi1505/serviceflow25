#!/bin/sh
# Copyright (C) 2026 group24 AG
# SPDX-License-Identifier: AGPL-3.0-only
#
# The CI of itsm, run in the tool container by scripts/ci-run.sh (Hauptplan E48, E52, E150):
#   scripts/ci-run.sh
# ci-run.sh gives it a PostgreSQL and a Valkey of its own, fresh for every run and removed
# afterwards, so that every run starts from the same state and leaves nothing behind.
#
# Every step runs, whatever the steps before did; a step that needs the result of a failed
# step is skipped. The end is a summary of every step and the failures with their reasons,
# so that one run shows everything (Pascal's decision 2026-11-02, E150).
set -u
cd "$(dirname "$0")/.."

steps=$(mktemp -d)
order=""

# run FN TITLE [NEEDS...]: runs the step function FN under its title. NEEDS are the functions
# of the steps it needs; when one of them did not pass, the step is skipped. The output goes
# to the screen and to a log for the summary.
run() {
  fn=$1
  title=$2
  shift 2
  order="$order $fn"
  printf '%s' "$title" > "$steps/$fn.title"
  printf '\n== %s\n' "$title"
  for need in "$@"; do
    if [ "$(cat "$steps/$need.status" 2>/dev/null)" != ok ]; then
      echo "skipped: needs \"$(cat "$steps/$need.title")\""
      echo skipped > "$steps/$fn.status"
      return 0
    fi
  done
  { ( set -e; "$fn" ); echo "$?" > "$steps/$fn.rc"; } 2>&1 | tee "$steps/$fn.log"
  if [ "$(cat "$steps/$fn.rc")" = 0 ]; then
    echo ok > "$steps/$fn.status"
  else
    echo "FAILED: $title"
    echo failed > "$steps/$fn.status"
  fi
}

license_headers() { scripts/check-license.sh; }

go_format() {
  unformatted=$(gofmt -l cmd internal db)
  if [ -n "$unformatted" ]; then
    echo "not gofmt-formatted:"; echo "$unformatted"; return 1
  fi
}

go_sqlc() { sqlc diff; }

go_oapi() {
  cp internal/httpapi/gen/api.gen.go /tmp/api.gen.go.before
  go tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml
  if ! cmp -s /tmp/api.gen.go.before internal/httpapi/gen/api.gen.go; then
    echo "internal/httpapi/gen/api.gen.go was out of date; regenerated, please commit it"; return 1
  fi
}

go_vet() { go vet ./...; }

go_lint() { golangci-lint run ./...; }

go_type_tables() { sh scripts/check-type-tables.sh; }

ci_database() {
  # The CI has a server of its own (postgres-ci, E150), started fresh by ci-run.sh; the
  # database is created here when the server does not bring it. Tests migrate it themselves.
  ci_db=$(printf '%s' "$ITSM_TEST_DATABASE_URL" | sed -E 's#^.*/([^/?]+)([?].*)?$#\1#')
  server_db=$(printf '%s' "$ITSM_TEST_DATABASE_URL" | sed -E 's#/[^/?]+([?]|$)#/postgres\1#')
  tries=0
  until psql "$server_db" -tAc "SELECT 1" >/dev/null 2>&1; do
    tries=$((tries + 1))
    if [ "$tries" -gt 60 ]; then echo "the database server of the CI is not reachable"; return 1; fi
    sleep 1
  done
  if [ "$(psql "$server_db" -tAc "SELECT 1 FROM pg_database WHERE datname = '$ci_db'")" != 1 ]; then
    psql "$server_db" -qc "CREATE DATABASE $ci_db"
    echo "created $ci_db"
  fi
  echo "databases on the server of the CI: $(psql "$server_db" -tAc "SELECT string_agg(datname, ', ' ORDER BY datname) FROM pg_database WHERE NOT datistemplate")"
}

# The packages that wait for events to become visible (the horizon, E87) or measure time run
# one after another, after everything else (E150): events, webhooks, jobs, the API tests and
# the load tool. Long transactions of a package running at the same time held the stream back
# and made those tests red without a fault in the code.
quiet_packages='^group24\.de/itsm/(internal/(events|webhooks|jobs|httpapi)|test/load)(/.*)?$'

go_test() {
  go list ./... | grep -Ev "$quiet_packages" > /tmp/pkgs-parallel
  go list ./... | grep -E "$quiet_packages" > /tmp/pkgs-quiet
  rc=0
  echo "-- side by side: $(wc -l < /tmp/pkgs-parallel) packages"
  xargs go test -race -count=1 < /tmp/pkgs-parallel || rc=1
  echo "-- one after another: $(tr '\n' ' ' < /tmp/pkgs-quiet)"
  xargs go test -race -count=1 -p 1 < /tmp/pkgs-quiet || rc=1
  return $rc
}

go_build() { go build -o /tmp/itsm ./cmd/itsm; }

# prepare_instance makes the database of the CI an instance for the load stage and the browser
# tests: migrated, with the packages core and probe (Hauptplan H13) and the accounts of the
# tests. Every step is repeatable, so both stages may call it.
prepare_instance() {
  export ITSM_DATABASE_URL="$ITSM_TEST_DATABASE_URL" ITSM_VALKEY_ADDR="$ITSM_TEST_VALKEY_ADDR"
  export ITSM_LOG_LEVEL=warn
  # LZ5 sends NDJSON streams, which the asynchronous bulk keeps in the file store (PL32).
  export ITSM_FILES_DIR=/tmp/itsm-load-files
  mkdir -p "$ITSM_FILES_DIR"
  # Its jobs run on a queue of their own (WF3): no other instance on this server takes them.
  export ITSM_BULK_QUEUE=bulkload
  /tmp/itsm migrate >/dev/null
  /tmp/itsm package install packages/core >/dev/null 2>&1
  /tmp/itsm package install packages/probe >/dev/null 2>&1
  # The load tool signs in with this account (test/load/internal/scenario); the password is the
  # tool's default for the test database only.
  # The password must not contain the username (PL28); an account from before gets it set.
  printf 'Messlauf-Pw7!' | /tmp/itsm user create load --display-name "Lasttest" >/dev/null 2>&1 ||
    printf 'Messlauf-Pw7!' | /tmp/itsm user passwd load >/dev/null 2>&1 || true
  /tmp/itsm user grant load admin >/dev/null 2>&1 || true
  # The browser test of UI8 needs a second person: a change by one's own account is not shown
  # as live, and one does not see oneself on a record.
  printf 'Zweite-Sicht8!' | /tmp/itsm user create kollegin --display-name "Kollegin" >/dev/null 2>&1 ||
    printf 'Zweite-Sicht8!' | /tmp/itsm user passwd kollegin >/dev/null 2>&1 || true
  /tmp/itsm user grant kollegin admin >/dev/null 2>&1 || true
  # The browser test of UI9 switches its account to English; an account of its own keeps the
  # other tests in German.
  printf 'Eigene-Wahl9!' | /tmp/itsm user create einsteller --display-name "Einsteller" >/dev/null 2>&1 ||
    printf 'Eigene-Wahl9!' | /tmp/itsm user passwd einsteller >/dev/null 2>&1 || true
  /tmp/itsm user grant einsteller admin >/dev/null 2>&1 || true
  # The browser test of UI29 switches its account to dark; an account of its own keeps the other
  # tests light.
  printf 'Dunkel-Hell29!' | /tmp/itsm user create gestalter --display-name "Gestalter" >/dev/null 2>&1 ||
    printf 'Dunkel-Hell29!' | /tmp/itsm user passwd gestalter >/dev/null 2>&1 || true
  /tmp/itsm user grant gestalter admin >/dev/null 2>&1 || true
}

load_quick() {
  prepare_instance
  export ITSM_HTTP_ADDR=127.0.0.1:18080
  /tmp/itsm serve --role=all &
  server=$!
  trap 'kill "$server" "${baseline:-}" 2>/dev/null || true' EXIT
  tries=0
  until /tmp/itsm healthcheck 2>/dev/null; do
    tries=$((tries + 1))
    if [ "$tries" -gt 60 ]; then echo "itsm did not start"; return 1; fi
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
      if [ "$tries" -gt 60 ]; then echo "baseline itsm did not start"; return 1; fi
      sleep 0.5
    done
    ab="--baseline-target=http://127.0.0.1:18081 --baseline-sha256=$(sha256sum "$BASELINE_BIN" | cut -d' ' -f1)"
  else
    echo "no baseline build below /var/lib/itsm-load: measuring the new build only"
  fi
  # shellcheck disable=SC2086
  go run ./test/load run --stage=quick --target=http://127.0.0.1:18080 $ab --wait-for-idle=120s --out=/tmp/load-quick.json
  go run ./test/load gate --stage=quick --result=/tmp/load-quick.json --baseline=test/load/baseline.json
}

web_install() { cd web && pnpm install --frozen-lockfile; }

# What itsm ships (the Go modules of the binary, the web packages of web/dist) has only allowed
# licenses (PL29, Hauptplan E15); the list and the exceptions are test/licenses/licenses.txt.
licenses() {
  (cd web && pnpm licenses list --prod --json >/tmp/web-licenses.json)
  go run ./test/licenses --web=/tmp/web-licenses.json
}

web_apigen() {
  cd web
  cp src/api/schema.d.ts /tmp/schema.d.ts
  pnpm api:gen >/dev/null
  cmp /tmp/schema.d.ts src/api/schema.d.ts
}

web_lint() { cd web && pnpm lint; }

web_format() { cd web && pnpm format:check; }

web_test() { cd web && pnpm test; }

web_build() { cd web && pnpm build; }

web_e2e() {
  # The same instance as the load stage, prepared again in case that stage was red.
  prepare_instance
  # All roles: the live test (UI8) needs the dispatcher of this server.
  ITSM_WEB_DIR=web/dist ITSM_HTTP_ADDR=127.0.0.1:18082 /tmp/itsm serve --role=all >/tmp/itsm-e2e.log 2>&1 &
  e2e_server=$!
  trap 'kill "$e2e_server" 2>/dev/null || true' EXIT
  for _ in $(seq 1 50); do
    curl -sf http://127.0.0.1:18082/healthz >/dev/null 2>&1 && break
    sleep 0.2
  done
  # The browser test of UI10 needs a user of the portal: her own records of probe, creating and
  # answering, no workplace. The command line only assigns roles, so the role comes over the
  # API of this server, signed in as the administrator load.
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
  # The browser test of UI27 narrows the role of a second person in the role editor: the role
  # e2e_rollen reads every probe and has the workplace; pruefer holds it.
  as_admin -X POST -d '{"name": "e2e_rollen", "label": {"de": "Rollenprüfung (Test)"}}' http://127.0.0.1:18082/api/v1/roles
  for permission in \
    '{"type_name": "probe.probe", "action": "read", "scope": "all"}' \
    '{"type_name": "core.ui", "action": "workplace", "scope": "all"}'; do
    as_admin -X PUT -d "$permission" http://127.0.0.1:18082/api/v1/roles/e2e_rollen/permissions
  done
  rm -f "$jar"
  printf 'Rollen-Pruef11!' | /tmp/itsm user create pruefer --display-name "Prüfer" >/dev/null 2>&1 ||
    printf 'Rollen-Pruef11!' | /tmp/itsm user passwd pruefer >/dev/null 2>&1 || true
  /tmp/itsm user grant pruefer e2e_rollen >/dev/null 2>&1 || true
  printf 'Nur-Portal10!' | /tmp/itsm user create nutzerin --display-name "Nutzerin" >/dev/null 2>&1 ||
    printf 'Nur-Portal10!' | /tmp/itsm user passwd nutzerin >/dev/null 2>&1 || true
  /tmp/itsm user grant nutzerin e2e_portal >/dev/null 2>&1 || true
  # The browser test of UI12 fills a form in English: a user of the portal of her own, so that
  # nutzerin stays German for the test of UI10.
  printf 'Portal-Sprache12!' | /tmp/itsm user create englisch --display-name "Englisch" >/dev/null 2>&1 ||
    printf 'Portal-Sprache12!' | /tmp/itsm user passwd englisch >/dev/null 2>&1 || true
  /tmp/itsm user grant englisch e2e_portal >/dev/null 2>&1 || true
  cd web
  ITSM_E2E_URL=http://127.0.0.1:18082 pnpm e2e
}

run license_headers "license headers"
run go_format "go: format"
run go_sqlc "go: sqlc code is up to date"
run go_oapi "go: generated API server is up to date (api/openapi.yaml, oapi-codegen)"
run go_vet "go: vet"
run go_lint "go: lint"
run go_type_tables "go: only records and query touch type tables (E34)"
run ci_database "go: the database of the CI"
run go_test "go: test" ci_database
run go_build "go: build"
run load_quick "load: quick stage, A/B against the baseline build (Hauptplan E32, E63, E69)" go_build ci_database
run web_install "web: install"
run licenses "licenses: Go modules and web packages that are shipped (Hauptplan E15)" web_install
run web_apigen "web: generated API types are up to date (api/openapi.yaml, openapi-typescript)" web_install
run web_lint "web: lint" web_install
run web_format "web: format" web_install
run web_test "web: test" web_install
run web_build "web: build" web_install
run web_e2e "web: browser tests (Playwright with axe, UI1)" web_build go_build ci_database

# The summary: every step with its outcome, then the failures with the lines that name them
# (failed tests, errors), so that one run says everything.
printf '\n== summary\n'
failed=0
for fn in $order; do
  status=$(cat "$steps/$fn.status")
  printf '%-8s %s\n' "$status" "$(cat "$steps/$fn.title")"
  [ "$status" = failed ] && failed=$((failed + 1))
done
if [ "$failed" -gt 0 ]; then
  printf '\n== failures\n'
  for fn in $order; do
    [ "$(cat "$steps/$fn.status")" = failed ] || continue
    printf -- '-- %s\n' "$(cat "$steps/$fn.title")"
    lines=$(grep -E '^(--- FAIL|FAIL[[:space:]]|[^[:space:]]+_test\.go:[0-9]+: )|^[[:space:]]*(✘|×)|Error:|error:|ERROR' "$steps/$fn.log" | grep -v '^go: downloading' | head -40)
    if [ -n "$lines" ]; then
      printf '%s\n' "$lines"
    else
      tail -n 15 "$steps/$fn.log"
    fi
  done
  printf '\nci: %d of %d steps failed\n' "$failed" "$(echo $order | wc -w)"
  rm -rf "$steps"
  exit 1
fi
rm -rf "$steps"
printf '\nci: all steps passed\n'
