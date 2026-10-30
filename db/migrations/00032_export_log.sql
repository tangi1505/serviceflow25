-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Exports in the security event log (UI5, Hauptplan E125): VS-NfD data leave the system with
-- them, so every export is kept with who, when, type, condition, columns, format and rows,
-- under a category of its own.

-- +goose Up
ALTER TABLE security_event DROP CONSTRAINT security_event_category_check;
ALTER TABLE security_event ADD CONSTRAINT security_event_category_check CHECK (category IN ('auth', 'change', 'data'));

-- +goose Down
DELETE FROM security_event WHERE category = 'data';
ALTER TABLE security_event DROP CONSTRAINT security_event_category_check;
ALTER TABLE security_event ADD CONSTRAINT security_event_category_check CHECK (category IN ('auth', 'change'));
