#!/bin/sh
# Copyright (C) 2026 group24 AG
# SPDX-License-Identifier: AGPL-3.0-only
#
# The core knows no package (SD21, SD-Plan E2: the service desk is configuration): no type of
# the packages sd, incident, request, catalog or knowledge is named in the code of the server or
# the interface. The check of the plan, without what only explains: comments, the API
# description generated from api/openapi.yaml, tests and their fakes.
set -eu
cd "$(dirname "$0")/.."

# grep, not git grep: the CI on ki-lab mirrors the code without .git.
hits=$(grep -rn -E '"(incident|request|catalog|knowledge|sd)\.' \
  --include='*.go' --include='*.ts' --include='*.tsx' internal web/src 2>/dev/null |
  grep -v -E '(_test\.go|\.test\.tsx?):' |
  grep -v -E '^(internal/httpapi/gen/|web/src/api/schema\.d\.ts:|web/src/test/)' |
  grep -v -E '^[^:]+:[0-9]+:[[:space:]]*(//|\*|/\*)' || true)
if [ -n "$hits" ]; then
  echo "the core names a type of a package (SD21); that belongs into the package:"
  echo "$hits"
  exit 1
fi
echo "core: no type of a package named"
