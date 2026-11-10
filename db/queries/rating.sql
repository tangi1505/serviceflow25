-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: UpsertRating :exec
INSERT INTO rating (type_name, record_id, person_id, helpful)
VALUES (sqlc.arg(type_name), sqlc.arg(record_id), sqlc.arg(person_id), sqlc.arg(helpful))
ON CONFLICT (type_name, record_id, person_id) DO UPDATE SET helpful = EXCLUDED.helpful, rated_at = now();

-- name: DeleteRating :exec
DELETE FROM rating
WHERE type_name = sqlc.arg(type_name) AND record_id = sqlc.arg(record_id) AND person_id = sqlc.arg(person_id);

-- name: RatingsOf :many
-- The sums of several records of one type; mine is the vote of the person, if any.
SELECT record_id,
       count(*) FILTER (WHERE helpful)::bigint     AS yes,
       count(*) FILTER (WHERE NOT helpful)::bigint AS no,
       count(*) FILTER (WHERE person_id = sqlc.narg(person_id)::uuid AND helpful)::bigint     AS mine_yes,
       count(*) FILTER (WHERE person_id = sqlc.narg(person_id)::uuid AND NOT helpful)::bigint AS mine_no
FROM rating
WHERE type_name = sqlc.arg(type_name) AND record_id = ANY(sqlc.arg(record_ids)::uuid[])
GROUP BY record_id;

-- name: DeleteRatingsOfRecord :exec
DELETE FROM rating WHERE type_name = sqlc.arg(type_name) AND record_id = sqlc.arg(record_id);
