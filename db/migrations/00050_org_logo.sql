-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- The logo of the organisation (UI39, UI-Plan E51): one for the light and one for the dark
-- mode, at the root or a tenant node. Not in the file store, because files hang at records and
-- their rights, while the sign-in page shows the logo of the root without a session. Small
-- (at most 256 KB, checked by internal/branding), PNG or SVG.

-- +goose Up
CREATE TABLE org_logo (
    path         ltree NOT NULL REFERENCES org_node(path) ON DELETE CASCADE,
    variant      text NOT NULL CHECK (variant IN ('light', 'dark')),
    content_type text NOT NULL CHECK (content_type IN ('image/png', 'image/svg+xml')),
    data         bytea NOT NULL,
    -- The hash of data: the address of the image changes with it, so a browser may keep it.
    sha          text NOT NULL,
    updated_at   timestamptz NOT NULL DEFAULT now(),
    updated_by   uuid,
    PRIMARY KEY (path, variant)
);

-- +goose Down
DROP TABLE org_logo;
