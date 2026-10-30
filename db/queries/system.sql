-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: DatabaseNow :one
-- Readiness check: proves that the pool can run a query, not only open a connection.
SELECT now()::timestamptz AS now;
