-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- An invited account (PL77) gets a random password nobody knows; when the mail of its
-- invitation does not go out, the account stays without an invitation. password_unknown says
-- so, until a password is set (by an invitation or as a one-time password), so the page does
-- not show it as an account with a one-time password.

-- +goose Up
ALTER TABLE account ADD COLUMN password_unknown boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE account DROP COLUMN password_unknown;
