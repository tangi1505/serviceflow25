-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Formatted text (UI7, Hauptplan E127): an entry of the timeline is plain text or HTML that
-- the server sanitized; the entries before stay text. An image pasted into an entry is
-- attached to the record with its entry, so that the image of a private entry is as private
-- as the entry.

-- +goose Up
ALTER TABLE entry ADD COLUMN format text NOT NULL DEFAULT 'text' CHECK (format IN ('text', 'html'));
ALTER TABLE attachment ADD COLUMN entry_id uuid;

-- +goose Down
ALTER TABLE attachment DROP COLUMN entry_id;
ALTER TABLE entry DROP COLUMN format;
