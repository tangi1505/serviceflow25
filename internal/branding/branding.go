// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package branding is the appearance of the organisation (UI39, UI-Plan E51): the name of the
// application, the accent colour, the main action in the accent and a logo for the light and the
// dark mode. Name, colour and main action are settings of the core (ui.app_name, ui.accent,
// ui.primary_accent) at the root or a tenant node; the logo lies in the table org_logo of the
// node's database. A tenant without its own value shows the one of the root. The sign-in page
// reads the appearance of the root without a session; only these values are readable so.
package branding

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/records"
)

// Keys of the settings of the appearance (packages/core/package.yaml).
const (
	KeyName    = "ui.app_name"
	KeyAccent  = "ui.accent"
	KeyPrimary = "ui.primary_accent"
)

// DefaultName is the name without a setting.
const DefaultName = "itsm"

// DefaultAccent is petrol, the accent without a setting.
const DefaultAccent = "#0B6E80"

// Variants are the logos of a node: one for each mode.
var Variants = []string{"light", "dark"}

// MaxLogo bounds a logo; it shows small in the header.
const MaxLogo = 256 << 10

// ErrInvalid is a logo the service refuses; the message says why.
var ErrInvalid = errors.New("branding: invalid")

// ErrNotFound is a logo that is not there.
var ErrNotFound = errors.New("branding: no logo")

// Settings reads a setting of a node with the node it comes from (org.Service.Setting).
type Settings func(ctx context.Context, path, key string) (json.RawMessage, string, error)

// Branding is the appearance at a node.
type Branding struct {
	Name          string `json:"name"`
	Accent        string `json:"accent"`
	PrimaryAccent bool   `json:"primary_accent"`
	// Logos holds the hash of each variant there is, the version of its address.
	Logos map[string]string `json:"-"`
}

// Logo is an image with its type and hash.
type Logo struct {
	ContentType string
	Data        []byte
	Sha         string
}

// Service reads and keeps the appearance.
type Service struct {
	db       *db.Router
	settings Settings
}

// New returns the service.
func New(rt *db.Router, settings Settings) *Service { return &Service{db: rt, settings: settings} }

// Of is the appearance at the node (the root or a tenant node).
func (s *Service) Of(ctx context.Context, node string) (Branding, error) {
	b := Branding{Name: DefaultName, Accent: DefaultAccent, Logos: map[string]string{}}
	if s.settings != nil {
		if raw, _, err := s.settings(ctx, node, KeyName); err != nil {
			return b, err
		} else if v := text(raw); v != "" {
			b.Name = v
		}
		if raw, _, err := s.settings(ctx, node, KeyAccent); err != nil {
			return b, err
		} else if v := text(raw); v != "" {
			b.Accent = AccentHex(v)
		}
		if raw, _, err := s.settings(ctx, node, KeyPrimary); err != nil {
			return b, err
		} else if raw != nil {
			_ = json.Unmarshal(raw, &b.PrimaryAccent)
		}
	}
	for _, v := range Variants {
		l, err := s.Logo(ctx, node, v)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return b, err
		}
		b.Logos[v] = l.Sha
	}
	return b, nil
}

func text(raw json.RawMessage) string {
	var v string
	if raw == nil || json.Unmarshal(raw, &v) != nil {
		return ""
	}
	return v
}

// named are the six accents a person chooses from (UI38) with their colour; ui.accent held
// their names before UI39.
var named = map[string]string{
	"petrol":   "#0B6E80",
	"steel":    "#1F5FA8",
	"indigo":   "#3F4BB8",
	"violet":   "#6B3FA0",
	"umber":    "#7A4E1D",
	"graphite": "#15171A",
}

// AccentHex is the colour of an accent given by name or as #rrggbb.
func AccentHex(v string) string {
	if c, ok := named[v]; ok {
		return c
	}
	return v
}

// Named are the names of the six accents and their colours.
func Named() map[string]string {
	out := make(map[string]string, len(named))
	for k, v := range named {
		out[k] = v
	}
	return out
}

// Logo reads the logo of the variant at the node, else at the root.
func (s *Service) Logo(ctx context.Context, node, variant string) (Logo, error) {
	for _, at := range []string{node, org.Root} {
		var l Logo
		err := s.db.For(at).QueryRow(ctx, `SELECT content_type, data, sha FROM org_logo WHERE path = $1::ltree AND variant = $2`,
			at, variant).Scan(&l.ContentType, &l.Data, &l.Sha)
		if err == nil {
			return l, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Logo{}, fmt.Errorf("branding: read logo: %w", err)
		}
		if at == org.Root {
			break
		}
	}
	return Logo{}, ErrNotFound
}

// SetLogo keeps an image as the logo of the variant at the node: PNG or SVG, at most MaxLogo.
func (s *Service) SetLogo(ctx context.Context, node, variant string, data []byte, actor records.Actor) (Logo, error) {
	if !validVariant(variant) {
		return Logo{}, fmt.Errorf("%w: the variant is light or dark", ErrInvalid)
	}
	if len(data) == 0 {
		return Logo{}, fmt.Errorf("%w: the image is empty", ErrInvalid)
	}
	if len(data) > MaxLogo {
		return Logo{}, fmt.Errorf("%w: the image has %d KB, at most 256 KB", ErrInvalid, len(data)>>10)
	}
	ctype := sniff(data)
	if ctype == "" {
		return Logo{}, fmt.Errorf("%w: the image must be a PNG or an SVG", ErrInvalid)
	}
	sum := sha256.Sum256(data)
	l := Logo{ContentType: ctype, Data: data, Sha: hex.EncodeToString(sum[:8])}
	by := (*uuid.UUID)(actor)
	_, err := s.db.For(node).Exec(ctx, `INSERT INTO org_logo (path, variant, content_type, data, sha, updated_by)
		VALUES ($1::ltree, $2, $3, $4, $5, $6)
		ON CONFLICT (path, variant) DO UPDATE SET content_type = EXCLUDED.content_type, data = EXCLUDED.data,
			sha = EXCLUDED.sha, updated_at = now(), updated_by = EXCLUDED.updated_by`,
		node, variant, ctype, data, l.Sha, by)
	if err != nil {
		return Logo{}, fmt.Errorf("branding: keep logo: %w", err)
	}
	return l, nil
}

// RemoveLogo takes the node's own logo of the variant away; the root's applies again.
func (s *Service) RemoveLogo(ctx context.Context, node, variant string) error {
	if !validVariant(variant) {
		return fmt.Errorf("%w: the variant is light or dark", ErrInvalid)
	}
	if _, err := s.db.For(node).Exec(ctx, `DELETE FROM org_logo WHERE path = $1::ltree AND variant = $2`, node, variant); err != nil {
		return fmt.Errorf("branding: remove logo: %w", err)
	}
	return nil
}

func validVariant(v string) bool { return v == "light" || v == "dark" }

// sniff names the type of an image by its content, never by what the client says: PNG by its
// signature, SVG by its root element.
func sniff(data []byte) string {
	if bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		return "image/png"
	}
	head := data
	if len(head) > 1024 {
		head = head[:1024]
	}
	head = bytes.TrimPrefix(head, []byte("\xef\xbb\xbf"))
	head = bytes.TrimSpace(head)
	if (bytes.HasPrefix(head, []byte("<?xml")) || bytes.HasPrefix(head, []byte("<svg")) || bytes.HasPrefix(head, []byte("<!--"))) &&
		bytes.Contains(data, []byte("<svg")) {
		return "image/svg+xml"
	}
	return ""
}
