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
#
# Two scopes (PL52, Hauptplan E162): ITSM_CI_SCOPE=full, the default, is the whole CI before a
# commit; ITSM_CI_SCOPE=part is the quick check while working: the Go packages that changed and
# all that use them, Vitest for the changed sources, the browser tests of the area, no load.
# What changed is listed in .ci-changed (ci-run.sh writes it from git: the changes since the
# last commit); without the file everything counts as changed. The load stage runs only when
# server code, migrations or the load tool changed.
set -u
cd "$(dirname "$0")/.."

steps=$(mktemp -d)
order=""
scope=${ITSM_CI_SCOPE:-full}
changed_file=.ci-changed
# changed REGEX: whether a changed path matches; everything matches without .ci-changed.
changed() {
  [ -f "$changed_file" ] || return 0
  grep -Eq "$1" "$changed_file"
}
server_re='^(cmd/|internal/|db/|go\.(mod|sum)$|test/load/)'
web_re='^web/'
go_re='(\.go$|^db/|^go\.(mod|sum)$|^api/|^packages/)'

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
  began=$(date +%s)
  { ( set -e; "$fn" ); echo "$?" > "$steps/$fn.rc"; } 2>&1 | tee "$steps/$fn.log"
  echo "$(($(date +%s) - began)) s" > "$steps/$fn.secs"
  if [ "$(cat "$steps/$fn.rc")" = 0 ]; then
    echo ok > "$steps/$fn.status"
  else
    echo "FAILED: $title"
    echo failed > "$steps/$fn.status"
  fi
}

# skip FN TITLE REASON: the step does not run in this scope or for these changes.
skip() {
  order="$order $1"
  printf '%s' "$2" > "$steps/$1.title"
  printf '\n== %s\nskipped: %s\n' "$2" "$3"
  echo skipped > "$steps/$1.status"
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

go_vet() {
  # shellcheck disable=SC2046
  go vet $(go_packages)
}

go_lint() {
  if [ "$scope" = part ]; then
    # golangci-lint takes directories: the affected packages as paths below the module.
    mod=$(go list -m)
    # shellcheck disable=SC2046
    golangci-lint run $(go_packages | sed "s#^$mod#.#")
  else
    golangci-lint run ./...
  fi
}

# go_packages are the packages to check: all, or in part scope those whose Go files changed and
# every package that imports one of them, directly or through others; package YAML and the API
# description count for the API tests, migrations and queries for the users of internal/db.
go_packages() {
  if [ "$scope" != part ] || [ ! -f "$changed_file" ]; then
    go list ./...
    return
  fi
  mod=$(go list -m)
  {
    grep -E '\.go$' "$changed_file" | while read -r f; do echo "$mod/$(dirname "$f")"; done
    changed '^db/' && printf '%s\n' "$mod/internal/db" "$mod/internal/db/queries"
    changed '^(packages/|api/)' && printf '%s\n' "$mod/internal/httpapi" "$mod/internal/meta" "$mod/cmd/itsm"
  } | sort -u > /tmp/ci-changed-pkgs
  go list -f '{{.ImportPath}} {{join .Deps " "}} {{join .TestImports " "}} {{join .XTestImports " "}}' ./... |
    awk 'BEGIN { while ((getline l < "/tmp/ci-changed-pkgs") > 0) c[l] = 1 }
      { for (i = 1; i <= NF; i++) if ($i in c) { print $1; break } }'
}

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
  # The CI database and one for each part of the API tests (PL52).
  for db in "$ci_db" $(seq 1 "$httpapi_parts" | sed "s/^/${ci_db}_h/"); do
    if [ "$(psql "$server_db" -tAc "SELECT 1 FROM pg_database WHERE datname = '$db'")" != 1 ]; then
      psql "$server_db" -qc "CREATE DATABASE $db"
      echo "created $db"
    fi
  done
  echo "databases on the server of the CI: $(psql "$server_db" -tAc "SELECT string_agg(datname, ', ' ORDER BY datname) FROM pg_database WHERE NOT datistemplate")"
}

# The packages that wait for events to become visible (the horizon, E87) or measure time run
# one after another, after everything else (E150): events, webhooks, jobs, the API tests and
# the load tool. Long transactions of a package running at the same time held the stream back
# and made those tests red without a fault in the code.
quiet_packages='^group24\.de/itsm/(internal/(events|webhooks|jobs|httpapi)|test/load)(/.*)?$'

# The API tests run in parts side by side (PL52): each part a database of its own on the server
# of the CI and a Valkey database of its own (keys and channels), so that their records and
# counters do not meet. Before, they ran one after another in 210 s. The tests that read the
# event stream itself (/api/v1/events) run alone afterwards: the horizon of the stream (E87)
# follows the oldest open transaction of the whole server, not of one database, so a part
# running beside them holds their stream back.
httpapi_parts=3

go_test() {
  go_packages > /tmp/pkgs-all
  # The tests of the load tool belong to the load stage: only when it runs (E162).
  if [ "$scope" = full ] && ! changed "$server_re"; then
    grep -Ev '/test/load(/|$)' /tmp/pkgs-all > /tmp/pkgs-run || true
  else
    cp /tmp/pkgs-all /tmp/pkgs-run
  fi
  grep -Ev "$quiet_packages" /tmp/pkgs-run > /tmp/pkgs-parallel || true
  grep -E "$quiet_packages" /tmp/pkgs-run | grep -v '/internal/httpapi$' > /tmp/pkgs-quiet || true
  # local: httpapi_test has an rc of its own; a shared one let a red package pass when the
  # API tests after it were green (found 2026-11-04 in SD16b, since PL52).
  local rc=0
  if [ -s /tmp/pkgs-parallel ]; then
    echo "-- side by side: $(wc -l < /tmp/pkgs-parallel) packages"
    xargs go test -race -count=1 < /tmp/pkgs-parallel || rc=1
  fi
  if [ -s /tmp/pkgs-quiet ]; then
    echo "-- one after another: $(tr '\n' ' ' < /tmp/pkgs-quiet)"
    xargs go test -race -count=1 -p 1 < /tmp/pkgs-quiet || rc=1
  fi
  if grep -q '/internal/httpapi$' /tmp/pkgs-run; then
    httpapi_test || rc=1
  fi
  if [ ! -s /tmp/pkgs-run ]; then
    echo "no Go package affected"
  fi
  return $rc
}

httpapi_test() {
  go test -race -c -o /tmp/httpapi.test ./internal/httpapi || return 1
  (cd internal/httpapi && /tmp/httpapi.test -test.list '.*') | grep '^Test' > /tmp/httpapi-tests
  # Tests that wait for events, through the stream of the API or with a dispatcher of their own
  # (PL47c), follow the horizon of the whole server (E87): they run alone.
  grep -l -e '/api/v1/events' -e 'RunDispatcher(' internal/httpapi/*_test.go | xargs sed -n 's/^func \(Test[A-Za-z0-9_]*\)(.*/\1/p' |
    sort -u > /tmp/httpapi-quiet
  for k in $(seq 1 "$httpapi_parts"); do : > "/tmp/httpapi-part-$k"; done
  i=0
  grep -vxF -f /tmp/httpapi-quiet /tmp/httpapi-tests | while read -r name; do
    echo "$name" >> "/tmp/httpapi-part-$((i % httpapi_parts + 1))"
    i=$((i + 1))
  done
  echo "-- internal/httpapi in $httpapi_parts parts side by side: $(cat /tmp/httpapi-part-* | wc -l) tests"
  started=$(date +%s)
  for k in $(seq 1 "$httpapi_parts"); do
    url=$(printf '%s' "$ITSM_TEST_DATABASE_URL" | sed -E "s#/([^/?]+)([?]|\$)#/\\1_h$k\\2#")
    run_re="^($(paste -sd'|' "/tmp/httpapi-part-$k"))\$"
    (
      cd internal/httpapi &&
        ITSM_TEST_DATABASE_URL="$url" ITSM_TEST_VALKEY_ADDR="$ITSM_TEST_VALKEY_ADDR/$k" \
          /tmp/httpapi.test -test.count=1 -test.timeout=15m -test.run "$run_re" > "/tmp/httpapi-part-$k.log" 2>&1 &&
        echo 0 > "/tmp/httpapi-part-$k.rc" || echo 1 > "/tmp/httpapi-part-$k.rc"
    ) &
  done
  wait
  local rc=0
  for k in $(seq 1 "$httpapi_parts"); do
    echo "-- part $k, $(wc -l < "/tmp/httpapi-part-$k") tests: $(tail -n 1 "/tmp/httpapi-part-$k.log")"
    if [ "$(cat "/tmp/httpapi-part-$k.rc")" != 0 ]; then
      cat "/tmp/httpapi-part-$k.log"
      rc=1
    fi
  done
  echo "-- the tests of the event stream alone: $(tr '\n' ' ' < /tmp/httpapi-quiet)"
  (
    cd internal/httpapi &&
      /tmp/httpapi.test -test.count=1 -test.timeout=15m -test.run "^($(paste -sd'|' /tmp/httpapi-quiet))\$" > /tmp/httpapi-quiet.log 2>&1
  ) || {
    cat /tmp/httpapi-quiet.log
    rc=1
  }
  echo "-- quiet: $(tail -n 1 /tmp/httpapi-quiet.log)"
  echo "-- internal/httpapi took $(($(date +%s) - started)) s"
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
  # The browser test of UI37 pins favourites and orders the areas; an account of its own keeps
  # the bar of the other tests as usual.
  printf 'Eigene-Ordnung37!' | /tmp/itsm user create anordner --display-name "Anordner" >/dev/null 2>&1 ||
    printf 'Eigene-Ordnung37!' | /tmp/itsm user passwd anordner >/dev/null 2>&1 || true
  /tmp/itsm user grant anordner admin >/dev/null 2>&1 || true
  # The browser test of UI39 changes the appearance of the organisation and gives it back at the
  # end; an account of its own keeps the others unaware of it.
  printf 'Eigene-Marke39!' | /tmp/itsm user create markenpfleger --display-name "Markenpfleger" >/dev/null 2>&1 ||
    printf 'Eigene-Marke39!' | /tmp/itsm user passwd markenpfleger >/dev/null 2>&1 || true
  /tmp/itsm user grant markenpfleger admin >/dev/null 2>&1 || true
}

# pin runs a command on the cores given (taskset), or anywhere when none are given. On ki-lab
# the measured servers and their database share 8 threads like the Prod server (a VM with 8
# virtual CPUs), and the load tool has 4 threads on cores of its own (PL71, Hauptplan E198:
# ITSM_LOAD_SERVER_CPUS, ITSM_LOAD_CLIENT_CPUS from compose/ci-kilab.yaml); the build and the
# tests keep all cores.
# A server started in the background with `pin ... &` runs in a subshell, and $! would be that
# subshell: kill would end it but not the server, which kept the pipe of tee open and the CI
# waiting for hours (2026-11-17). So pin replaces itself with the command (exec); called in the
# foreground it runs in a subshell of its own for the same reason.
pin() {
  cpus=$1
  shift
  if [ -n "$cpus" ]; then
    exec taskset -c "$cpus" "$@"
  fi
  exec "$@"
}

load_quick() {
  prepare_instance
  export ITSM_HTTP_ADDR=127.0.0.1:18080
  pin "${ITSM_LOAD_SERVER_CPUS:-}" /tmp/itsm serve --role=all &
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
    pin "${ITSM_LOAD_SERVER_CPUS:-}" env ITSM_HTTP_ADDR=127.0.0.1:18081 "$BASELINE_BIN" serve --role=all &
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
  go build -o /tmp/load ./test/load
  # shellcheck disable=SC2086
  (pin "${ITSM_LOAD_CLIENT_CPUS:-}" /tmp/load run --stage=quick --target=http://127.0.0.1:18080 $ab --wait-for-idle=120s --out=/tmp/load-quick.json)
  /tmp/load gate --stage=quick --result=/tmp/load-quick.json --baseline=test/load/baseline.json
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

web_test() {
  cd web
  if [ "$scope" = part ] && [ -f "../$changed_file" ]; then
    files=$(grep -E '^web/src/' "../$changed_file" | sed 's#^web/##' | while read -r f; do [ -f "$f" ] && echo "$f"; done)
    if [ -z "$files" ]; then
      echo "no changed source of the web: no Vitest"
      return 0
    fi
    # The tests that import a changed file, directly or through others.
    # shellcheck disable=SC2086
    pnpm exec vitest related --run --passWithNoTests $files
  else
    pnpm test
  fi
}

# e2e_specs prints the browser tests of a part run: the changed ones and those of the areas
# that changed; ALL for shared code (client, texts, the fake of the tests) or every spec in a
# full run; nothing when the web did not change. Server code alone brings the walk through
# the records as a smoke test.
e2e_specs() {
  if [ "$scope" != part ] || [ ! -f "$changed_file" ]; then
    echo ALL
    return
  fi
  if grep -Eq '^web/(src/(api|i18n|test)/|package\.json|playwright\.config\.ts|vite\.config\.ts)' "$changed_file"; then
    echo ALL
    return
  fi
  {
    grep -E '^web/e2e/.*\.spec\.ts$' "$changed_file" | sed 's#^web/##'
    while read -r pattern specs; do
      if grep -Eq "$pattern" "$changed_file"; then
        for spec in $specs; do echo "e2e/$spec.spec.ts"; done
      fi
    done <<'MAP'
^web/src/render/(RecordPage|RecordTabs|Relations|Timeline|Form|Sidebar|Transitions|RefPicker|RefName|RichEditor|Presence|fields) records relations live
^web/src/render/(List|ListPreview|Preview|Filters|Batch|Export|DeadlineCell|SearchPage) records deadline scroll
^web/src/(render/FormBuilder|portal/) forms form-conditions portal
^web/src/render/Workflow workflow
^web/src/render/Role roles
^web/src/render/(Customize|TypePage) customize
^web/src/app/ login frame command settings design sequence
^web/src/ui/ design frame
^(cmd|internal|db)/ records
MAP
  } | sort -u | while read -r f; do [ -f "web/$f" ] && echo "$f"; done
}

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
  rc=0
  # shellcheck disable=SC2086
  if [ "$specs" = ALL ]; then
    ITSM_E2E_URL=http://127.0.0.1:18082 pnpm e2e || rc=$?
  else
    echo "-- browser tests of this part run: $specs"
    ITSM_E2E_URL=http://127.0.0.1:18082 pnpm e2e $specs || rc=$?
  fi
  # A red run shows what the server logged as an error (UI63: a 500 whose log went with the
  # container).
  if [ "$rc" != 0 ]; then
    echo "-- errors in the log of the server"
    grep -E '"level":"(ERROR|WARN)"|level=(ERROR|WARN)' /tmp/itsm-e2e.log | tail -20 || true
  fi
  return "$rc"
}

echo "ci: scope $scope"
if [ -f "$changed_file" ]; then
  echo "ci: $(wc -l < "$changed_file") changed files since the last commit"
else
  echo "ci: no list of changed files, everything counts as changed"
fi
specs=$(e2e_specs | tr '\n' ' ' | sed 's/ $//')
part_go() { [ "$scope" = full ] || changed "$go_re"; }
part_web() { [ "$scope" = full ] || changed "$web_re" || [ -n "$specs" ]; }
nogo="no change to Go code, migrations, packages or the API since the last commit"
noweb="no change to the web since the last commit"

run license_headers "license headers"
if part_go; then
  run go_format "go: format"
  run go_sqlc "go: sqlc code is up to date"
  run go_oapi "go: generated API server is up to date (api/openapi.yaml, oapi-codegen)"
  run go_vet "go: vet"
  run go_lint "go: lint"
  run go_type_tables "go: only records and query touch type tables (E34)"
else
  for step in go_format:"go: format" go_sqlc:"go: sqlc code" go_oapi:"go: generated API server"     go_vet:"go: vet" go_lint:"go: lint" go_type_tables:"go: type tables (E34)"; do
    skip "${step%%:*}" "${step#*:}" "$nogo"
  done
fi
run ci_database "go: the database of the CI"
if part_go; then
  run go_test "go: test" ci_database
else
  skip go_test "go: test" "$nogo"
fi
run go_build "go: build"
if [ "$scope" = part ]; then
  skip load_quick "load: quick stage" "the part run measures nothing (Hauptplan E162)"
elif changed "$server_re"; then
  run load_quick "load: quick stage, A/B against the baseline build (Hauptplan E32, E63, E69)" go_build ci_database
else
  skip load_quick "load: quick stage" "no change to server code, migrations or the load tool since the last commit (Hauptplan E162)"
fi
if part_web; then
  run web_install "web: install"
  if [ "$scope" = full ] || changed '^(go\.(mod|sum)|web/package\.json|web/pnpm-lock\.yaml)$'; then
    run licenses "licenses: Go modules and web packages that are shipped (Hauptplan E15)" web_install
  else
    skip licenses "licenses" "no change to the modules or packages"
  fi
  run web_apigen "web: generated API types are up to date (api/openapi.yaml, openapi-typescript)" web_install
  run web_lint "web: lint" web_install
  run web_format "web: format" web_install
  run web_test "web: test" web_install
  if [ -n "$specs" ]; then
    run web_build "web: build" web_install
    run web_e2e "web: browser tests (Playwright with axe, UI1)" web_build go_build ci_database
  else
    skip web_e2e "web: browser tests" "no browser test for these changes"
  fi
else
  skip web_test "web: test" "$noweb"
  skip web_e2e "web: browser tests" "$noweb"
fi

# The summary: every step with its outcome and its time (PL52), then the failures with the
# lines that name them (failed tests, errors), so that one run says everything.
printf '\n== summary\n'
failed=0
for fn in $order; do
  status=$(cat "$steps/$fn.status")
  printf '%-8s %6s  %s\n' "$status" "$(cat "$steps/$fn.secs" 2>/dev/null)" "$(cat "$steps/$fn.title")"
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
