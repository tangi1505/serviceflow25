#!/bin/sh
# Copyright (C) 2026 group24 AG
# SPDX-License-Identifier: AGPL-3.0-only
#
# This program is free software: you can redistribute it and/or modify it under the terms of
# the GNU Affero General Public License as published by the Free Software Foundation, version 3.
# It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

# The instance of the walk-through of milestone 1 (SD21): a database of its own next to the one
# of the other browser tests, freshly migrated, with the packages of the service desk installed
# and nothing else; one administrator; the rebuilt identity provider of the tests (test/fakeidp,
# Pascal's decision 2026-11-20) and a server that signs in through it. Everything else (roles,
# login rules, approvals, articles) the browser test configures itself, as a customer would.
#
#   sd21-instance.sh start   prepare and start; prints the addresses
#   sd21-instance.sh stop    stop provider and server
#
# Environment: ITSM_TEST_DATABASE_URL and ITSM_TEST_VALKEY_ADDR (the CI's), ITSM_BIN and
# FAKEIDP_BIN (default /tmp/itsm, /tmp/fakeidp), SD21_LISTEN (default 127.0.0.1; 0.0.0.0 for a
# container with published ports), SD21_HOST (how the browser reaches both, default 127.0.0.1).
set -eu
bin=${ITSM_BIN:-/tmp/itsm}
idp_bin=${FAKEIDP_BIN:-/tmp/fakeidp}
listen=${SD21_LISTEN:-127.0.0.1}
host=${SD21_HOST:-127.0.0.1}
idp_port=18083
port=18084
pids=/tmp/sd21-instance.pids

stop() {
  if [ -f "$pids" ]; then
    # shellcheck disable=SC2046
    kill $(cat "$pids") 2>/dev/null || true
    rm -f "$pids"
  fi
}

case "${1:-}" in
stop)
  stop
  exit 0
  ;;
start) ;;
*)
  echo "usage: sd21-instance.sh start|stop" >&2
  exit 2
  ;;
esac
stop

db_url=$(printf '%s' "$ITSM_TEST_DATABASE_URL" | sed -E 's#/([^/?]+)([?]|$)#/\1_sd21\2#')
db=$(printf '%s' "$db_url" | sed -E 's#^.*/([^/?]+)([?].*)?$#\1#')
server_db=$(printf '%s' "$ITSM_TEST_DATABASE_URL" | sed -E 's#/[^/?]+([?]|$)#/postgres\1#')
psql "$server_db" -qc "DROP DATABASE IF EXISTS $db WITH (FORCE)"
psql "$server_db" -qc "CREATE DATABASE $db"

export ITSM_DATABASE_URL="$db_url"
# Valkey database 15: the API tests take 1 to 3, the other browser tests 0.
export ITSM_VALKEY_ADDR="$ITSM_TEST_VALKEY_ADDR/15"
export ITSM_LOG_LEVEL=warn
export ITSM_FILES_DIR=/tmp/itsm-sd21-files
rm -rf "$ITSM_FILES_DIR"
mkdir -p "$ITSM_FILES_DIR"
"$bin" migrate >/dev/null
for p in core sd incident request catalog knowledge; do
  "$bin" package install "packages/$p" >/dev/null
done
# The administrator who configures; the password is for this test database only.
printf 'Durchgang-Pw21!' | "$bin" user create sd21admin --display-name "Einrichtung" >/dev/null
"$bin" user grant sd21admin admin >/dev/null

"$idp_bin" -addr "$listen:$idp_port" -issuer "http://$host:$idp_port" >/tmp/fakeidp.log 2>&1 &
echo $! > "$pids"
ITSM_HTTP_ADDR="$listen:$port" ITSM_PUBLIC_URL="http://$host:$port" ITSM_WEB_DIR=web/dist \
  ITSM_OIDC_ISSUER="http://$host:$idp_port" \
  ITSM_OIDC_CLIENT_ID=itsm-test ITSM_OIDC_CLIENT_SECRET=itsm-test-secret ITSM_OIDC_NAME=Testanbieter \
  "$bin" serve --role=all >/tmp/itsm-sd21.log 2>&1 &
echo $! >> "$pids"
tries=0
until curl -sf "http://127.0.0.1:$port/healthz" >/dev/null 2>&1; do
  tries=$((tries + 1))
  if [ "$tries" -gt 100 ]; then
    echo "the server of SD21 did not start:"
    tail -20 /tmp/itsm-sd21.log
    exit 1
  fi
  sleep 0.2
done
echo "sd21: server http://$host:$port, provider http://$host:$idp_port, database $db"
