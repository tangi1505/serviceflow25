-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: TranslationsOf :many
SELECT record_id, field, language, value
FROM translation
WHERE type_name = sqlc.arg(type_name) AND record_id = ANY(sqlc.arg(record_ids)::uuid[]);

-- name: UpsertTranslation :exec
INSERT INTO translation (type_name, record_id, field, language, value)
VALUES (sqlc.arg(type_name), sqlc.arg(record_id), sqlc.arg(field), sqlc.arg(language), sqlc.arg(value))
ON CONFLICT (type_name, record_id, field, language) DO UPDATE SET value = excluded.value;

-- name: DeleteTranslation :exec
DELETE FROM translation
WHERE type_name = sqlc.arg(type_name) AND record_id = sqlc.arg(record_id)
  AND field = sqlc.arg(field) AND language = sqlc.arg(language);

-- name: DeleteTranslationsOfRecord :exec
DELETE FROM translation WHERE type_name = sqlc.arg(type_name) AND record_id = sqlc.arg(record_id);
