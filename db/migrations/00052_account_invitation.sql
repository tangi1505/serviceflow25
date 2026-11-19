-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Accounts in the interface (PL77, UI-Plan E63): the last sign-in of a local account, and the
-- invitation of a new local account by mail. An invitation holds the hash of a one-time key
-- (the key itself is only in the mail), the address it went to and until when it holds; one
-- per account, a new one replaces it, using it removes it.

-- +goose Up
ALTER TABLE account ADD COLUMN last_login_at timestamptz;

CREATE TABLE account_invitation (
    account_id uuid PRIMARY KEY REFERENCES account (id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    email      text NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid
);

-- +goose Down
DROP TABLE account_invitation;
ALTER TABLE account DROP COLUMN last_login_at;
