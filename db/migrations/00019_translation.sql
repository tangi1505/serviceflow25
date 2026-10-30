-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Translated field values (Hauptplan KF19, E98; PL21; GLPI ItemTranslation): the column of a
-- translatable text field holds the German value, the fallback of every language; the other
-- languages live here, one row per record, field and language. Filters, sorting, unique
-- values and rules work on the German value.

-- +goose Up
CREATE TABLE translation (
    type_name text NOT NULL,
    record_id uuid NOT NULL,
    field     text NOT NULL,
    language  text NOT NULL CHECK (language ~ '^[a-z]{2}$' AND language <> 'de'),
    value     text NOT NULL,
    PRIMARY KEY (type_name, record_id, field, language)
);

-- +goose Down
DROP TABLE translation;
