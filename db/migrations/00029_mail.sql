-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Notifications by mail (WF15): a mail that fails on its last attempt is kept with the other
-- failures of the dispatcher (PL41).

-- +goose Up
ALTER TABLE delivery_failure DROP CONSTRAINT delivery_failure_kind_check;
ALTER TABLE delivery_failure ADD CONSTRAINT delivery_failure_kind_check CHECK (kind IN ('webhook', 'rule', 'mail'));

-- +goose Down
DELETE FROM delivery_failure WHERE kind = 'mail';
ALTER TABLE delivery_failure DROP CONSTRAINT delivery_failure_kind_check;
ALTER TABLE delivery_failure ADD CONSTRAINT delivery_failure_kind_check CHECK (kind IN ('webhook', 'rule'));
