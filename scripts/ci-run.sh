#!/bin/sh
# Copyright (C) 2026 group24 AG
# SPDX-License-Identifier: AGPL-3.0-only
#
# Runs the CI (scripts/ci.sh) in the tool container with a PostgreSQL and a Valkey of its own
# (Hauptplan E150, Pascal's decision 2026-11-02): both start fresh for every run (their data
# lives in memory, profile ci of compose.yaml) and are removed afterwards, so that every run
# starts from the same state and leaves nothing behind. The instance on 8092 and its database
# are not touched. Usage, from the directory of the project:
#   scripts/ci-run.sh           the whole CI, before every commit
#   scripts/ci-run.sh --part    the quick check while working (PL52, Hauptplan E162)
#   scripts/ci-run.sh --all     the whole CI as if everything had changed (load stage included)
# What changed since the last commit goes to .ci-changed for ci.sh; a copy without .git (the
# copy in WSL) brings its own .ci-changed, written by whoever synced it.
set -u
cd "$(dirname "$0")/.."

scope=full
case "${1:-}" in
  --part) scope=part ;;
  --all) rm -f .ci-changed ;;
  "") ;;
  *) echo "usage: scripts/ci-run.sh [--part|--all]"; exit 2 ;;
esac
if [ "${1:-}" != --all ] && [ -d .git ]; then
  { git diff --name-only HEAD; git ls-files --others --exclude-standard; } | sort -u > .ci-changed
fi

ci="docker compose --profile ci"
cleanup() { $ci rm -sfv postgres-ci valkey-ci >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "ci: fresh PostgreSQL and Valkey for this run"
if ! $ci up -d --wait --force-recreate --renew-anon-volumes postgres-ci valkey-ci; then
  echo "ci: the servers of the CI did not start"
  exit 1
fi
docker compose run --rm -T -e ITSM_CI_SCOPE="$scope" tools scripts/ci.sh
rc=$?
echo "ci: removing the servers of this run"
exit $rc
