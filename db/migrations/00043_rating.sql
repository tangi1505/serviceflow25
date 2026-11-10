-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Ratings (SD18, SD-Plan N14): one core table for the records of every type with the trait
-- rating. A person says once whether a record helped, yes or no, and may change it; the
-- record keeps its version. The ratings go with the record when it is purged.

-- +goose Up
CREATE TABLE rating (
    type_name  text        NOT NULL,
    record_id  uuid        NOT NULL,
    person_id  uuid        NOT NULL,
    helpful    boolean     NOT NULL,
    rated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (type_name, record_id, person_id)
);

-- +goose Down
DROP TABLE rating;
