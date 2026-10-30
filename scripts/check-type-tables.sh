#!/bin/sh
# Copyright (C) 2026 group24 AG
# SPDX-License-Identifier: AGPL-3.0-only
#
# Only records and query may touch type tables (Hauptplan E34, architektur.md §5; PL8): any Go
# string literal naming a table rec_<package>_<type> elsewhere breaks the CI. Tests are free
# to set up and inspect tables.
set -eu
cd "$(dirname "$0")/.."

hits=$(grep -rn --include='*.go' -E '"[^"]*rec_[a-z]' internal cmd test 2>/dev/null \
  | grep -v -E '^internal/(records|query)/' \
  | grep -v -E '_test\.go:' || true)
if [ -n "$hits" ]; then
  echo "SQL on type tables belongs into internal/records or internal/query (E34):"
  echo "$hits"
  exit 1
fi
echo "type tables: only records and query touch them"
