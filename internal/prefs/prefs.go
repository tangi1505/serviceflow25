// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package prefs keeps the settings of a person (UI9; Hauptplan E131): language, time zone,
// theme and the presets of a type when creating. The language lives at the person
// (core.person.language), which the mails use too; the rest in the core table
// account_setting. What a person did not set comes from the settings of the organisation
// tree at the person's unit (ui.language, ui.time_zone, ui.theme, inherited), else from the
// default. Every value says where it comes from.
package prefs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
)

// Keys of the global defaults at the organisation tree.
const (
	GlobalLanguage = "ui.language"
	GlobalTimeZone = "ui.time_zone"
	GlobalTheme    = "ui.theme"
	// GlobalColorScheme is light, dark or system (UI29, UI-Plan E24).
	GlobalColorScheme = "ui.color_scheme"
	// GlobalDensity is compact or normal, the row height of lists (UI30, UI-Plan E35).
	GlobalDensity = "ui.density"
	// GlobalStartDashboard is the dashboard on the start page of the workplace (UI13).
	GlobalStartDashboard = "ui.start_dashboard"
	// GlobalAccent is the accent colour of the organisation (UI38, UI-Plan E49): one of Accents.
	GlobalAccent = "ui.accent"
)

// Accents are the six accent colours a person chooses from (UI38, UI-Plan §2.3): without red,
// yellow and green, which mean deadline and state everywhere.
var Accents = []string{"petrol", "steel", "indigo", "violet", "umber", "graphite"}

// DefaultAccent is the accent without a setting of the person or the tree.
const DefaultAccent = "petrol"

func isAccent(v string) bool { return slices.Contains(Accents, v) }

// StartNone is the own start page without a dashboard: the greeting only (UI13).
const StartNone = "none"

// Defaults when neither the person nor the tree says anything.
const (
	DefaultLanguage = "de"
	DefaultTimeZone = "Europe/Berlin"
	DefaultTheme    = "neutral"
	// DefaultColorScheme follows the system of the device (prefers-color-scheme).
	DefaultColorScheme = "system"
	// DefaultDensity is the normal row height of 52 px.
	DefaultDensity = "normal"
)

// Sources of a value besides a node path.
const (
	SourceOwn     = "own"
	SourceDefault = "default"
)

// Languages are the languages of the interface.
var Languages = []string{"de", "en"}

var themeRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,30}$`)

// isColorScheme says whether v is light, dark or system.
func isColorScheme(v string) bool { return v == "light" || v == "dark" || v == "system" }

// isDensity says whether v is compact or normal.
func isDensity(v string) bool { return v == "compact" || v == "normal" }

// isNav is the width of the navigation (UI36): wide with names, narrow with symbols only.
func isNav(v string) bool { return v == "wide" || v == "narrow" }

// DefaultNav is the width of the navigation without an own setting.
const DefaultNav = "wide"

// The arrangement of the navigation (UI37, UI-Plan E50): lists of keys the interface knows
// (areas such as package.incident, entries such as package.incident/incident.incident). The
// server only bounds them; a key without its area or entry is passed by in the interface.
var navKeyRe = regexp.MustCompile(`^[A-Za-z0-9_.:/-]{1,200}$`)

// MaxNavKeys bounds each list of the arrangement.
const MaxNavKeys = 200

// navLists are the settings that are lists of keys.
var navLists = []string{"nav_order", "nav_hidden", "favorites"}

// Value is one setting with where it comes from: own, a node path, or default.
type Value struct {
	Value  string `json:"value"`
	Source string `json:"source"`
}

// Prefs are the effective settings of a person.
type Prefs struct {
	Language Value `json:"language"`
	TimeZone Value `json:"time_zone"`
	Theme    Value `json:"theme"`
	// ColorScheme is light, dark or system (UI29).
	ColorScheme Value `json:"color_scheme"`
	// Density is compact or normal (UI30).
	Density Value `json:"density"`
	// StartDashboard is the id of the dashboard on the start page, or "none" for the greeting
	// only; empty without one (UI13).
	StartDashboard Value `json:"start_dashboard"`
	// Accent is the accent colour, one of Accents (UI38).
	Accent Value `json:"accent"`
	// Nav is the width of the navigation, wide or narrow (UI36); only the person sets it.
	Nav Value `json:"nav"`
	// NavOrder is the own order of the areas of the navigation, NavHidden the areas left out,
	// Favorites the entries pinned under Favoriten in their order (UI37); only the person sets
	// them, empty without.
	NavOrder  []string                  `json:"nav_order"`
	NavHidden []string                  `json:"nav_hidden"`
	Favorites []string                  `json:"favorites"`
	Defaults  map[string]map[string]any `json:"defaults"`
}

// Settings reads a setting of the tree with the node it comes from.
type Settings func(ctx context.Context, path, key string) (json.RawMessage, string, error)

// Service keeps the settings.
type Service struct {
	db       *db.Router
	rec      *records.Service
	settings Settings
}

// New returns the service.
func New(rt *db.Router, rec *records.Service, settings Settings) *Service {
	return &Service{db: rt, rec: rec, settings: settings}
}

// tq is the query handle in the database of the account (PL47b): its settings live with it.
func (s *Service) tq(ctx context.Context, account uuid.UUID) *queries.Queries {
	pool, _, err := s.db.Account(ctx, account)
	if err != nil {
		pool = s.db.Control()
	}
	return queries.New(pool)
}

func system(ctx context.Context) context.Context { return authz.WithAccess(ctx, authz.System()) }

// person reads the person of the account, nil without one.
func (s *Service) person(ctx context.Context, personID *uuid.UUID) (records.Record, error) {
	if personID == nil {
		return nil, nil
	}
	p, err := s.rec.Get(system(ctx), "core.person", *personID)
	if errors.Is(err, records.ErrNotFound) {
		return nil, nil
	}
	return p, err
}

// global reads a default of the tree at the node, as a string.
func (s *Service) global(ctx context.Context, node, key string) (string, string) {
	if s.settings == nil {
		return "", ""
	}
	raw, src, err := s.settings(ctx, node, key)
	if err != nil || raw == nil {
		return "", ""
	}
	var v string
	if json.Unmarshal(raw, &v) != nil || v == "" {
		return "", ""
	}
	return v, src
}

// Get returns the effective settings of the account.
func (s *Service) Get(ctx context.Context, account uuid.UUID, personID *uuid.UUID) (Prefs, error) {
	rows, err := s.tq(ctx, account).AccountSettings(ctx, account)
	if err != nil {
		return Prefs{}, fmt.Errorf("prefs: read: %w", err)
	}
	own := map[string]json.RawMessage{}
	out := Prefs{Defaults: map[string]map[string]any{}}
	for _, r := range rows {
		if typeName, ok := strings.CutPrefix(r.Key, "defaults."); ok {
			var m map[string]any
			if json.Unmarshal(r.Value, &m) == nil {
				out.Defaults[typeName] = m
			}
			continue
		}
		own[r.Key] = r.Value
	}
	p, err := s.person(ctx, personID)
	if err != nil {
		return Prefs{}, err
	}
	node := "root"
	if path, _ := p["org_path"].(string); path != "" {
		node = path
	}
	pick := func(ownValue, key, fallback string) Value {
		if ownValue != "" {
			return Value{ownValue, SourceOwn}
		}
		if v, src := s.global(ctx, node, key); v != "" {
			return Value{v, src}
		}
		return Value{fallback, SourceDefault}
	}
	str := func(key string) string {
		var v string
		_ = json.Unmarshal(own[key], &v)
		return v
	}
	lang, _ := p["language"].(string)
	if p == nil {
		lang = str("language")
	}
	out.Language = pick(lang, GlobalLanguage, DefaultLanguage)
	out.TimeZone = pick(str("time_zone"), GlobalTimeZone, DefaultTimeZone)
	// The theme is the organisation's only (UI38, UI-Plan E49): an own value of before is passed by.
	out.Theme = pick("", GlobalTheme, DefaultTheme)
	scheme := pick(str("color_scheme"), GlobalColorScheme, DefaultColorScheme)
	if !isColorScheme(scheme.Value) {
		// A wrong value at the tree falls back, the interface stays usable.
		scheme = Value{DefaultColorScheme, SourceDefault}
	}
	out.ColorScheme = scheme
	density := pick(str("density"), GlobalDensity, DefaultDensity)
	if !isDensity(density.Value) {
		density = Value{DefaultDensity, SourceDefault}
	}
	out.Density = density
	out.StartDashboard = pick(str("start_dashboard"), GlobalStartDashboard, "")
	accent := pick(str("accent"), GlobalAccent, DefaultAccent)
	if !isAccent(accent.Value) {
		accent = Value{DefaultAccent, SourceDefault}
	}
	out.Accent = accent
	out.Nav = Value{DefaultNav, SourceDefault}
	if v := str("nav"); isNav(v) {
		out.Nav = Value{v, SourceOwn}
	}
	list := func(key string) []string {
		v := []string{}
		_ = json.Unmarshal(own[key], &v)
		if v == nil {
			v = []string{}
		}
		return v
	}
	out.NavOrder = list("nav_order")
	out.NavHidden = list("nav_hidden")
	out.Favorites = list("favorites")
	return out, nil
}

// Set changes the settings given; null takes the own value away, so the global one applies.
// defaults takes per type an object of field values, or null.
func (s *Service) Set(ctx context.Context, account uuid.UUID, personID *uuid.UUID, input map[string]any) error {
	var ps []records.Problem
	bad := func(field, code, msg string) {
		ps = append(ps, records.Problem{Field: field, Code: code, Message: msg})
	}
	type write struct {
		key   string
		value any // nil removes
	}
	var writes []write
	text := func(name string, check func(string) bool, hint string) {
		v, given := input[name]
		if !given {
			return
		}
		if v == nil {
			writes = append(writes, write{name, nil})
			return
		}
		str, ok := v.(string)
		if !ok || !check(str) {
			bad(name, "invalid", hint)
			return
		}
		writes = append(writes, write{name, str})
	}
	for name := range input {
		switch name {
		case "language", "time_zone", "theme", "color_scheme", "density", "start_dashboard", "accent", "nav", "nav_order", "nav_hidden", "favorites", "defaults":
		default:
			bad(name, "unknown", "is no setting; settings are language, time_zone, theme, color_scheme, density, start_dashboard, accent, nav, nav_order, nav_hidden, favorites and defaults")
		}
	}
	// The lists of the navigation (UI37): keys without repeats; null or an empty list takes
	// them away.
	for _, name := range navLists {
		v, given := input[name]
		if !given {
			continue
		}
		items, ok := v.([]any)
		if v != nil && !ok {
			bad(name, "type", "must be a list of keys or null")
			continue
		}
		if len(items) == 0 {
			writes = append(writes, write{name, nil})
			continue
		}
		if len(items) > MaxNavKeys {
			bad(name, "invalid", fmt.Sprintf("holds at most %d keys", MaxNavKeys))
			continue
		}
		keys := make([]string, 0, len(items))
		seen := map[string]bool{}
		for _, it := range items {
			k, ok := it.(string)
			if !ok || !navKeyRe.MatchString(k) || seen[k] {
				bad(name, "invalid", "must hold distinct keys of letters, digits and _ . : / - (at most 200 characters)")
				keys = nil
				break
			}
			seen[k] = true
			keys = append(keys, k)
		}
		if keys != nil {
			writes = append(writes, write{name, keys})
		}
	}
	text("language", func(v string) bool { return v == "de" || v == "en" }, "must be de or en")
	text("time_zone", func(v string) bool { _, err := time.LoadLocation(v); return err == nil && v != "" && v != "Local" }, "must be an IANA time zone such as Europe/Berlin")
	// A person sets no theme any more (E49); null still takes an own value of before away.
	if v, given := input["theme"]; given && v != nil {
		bad("theme", "invalid", "is set by the organisation (ui.theme), not by a person")
	} else {
		text("theme", themeRe.MatchString, "must be the name of a theme")
	}
	text("color_scheme", isColorScheme, "must be light, dark or system")
	text("density", isDensity, "must be compact or normal")
	text("nav", isNav, "must be wide or narrow")
	text("accent", isAccent, "must be one of petrol, steel, indigo, violet, umber, graphite")
	text("start_dashboard", func(v string) bool {
		if v == StartNone {
			return true
		}
		id, err := uuid.Parse(v)
		if err != nil {
			return false
		}
		_, err = s.rec.Get(ctx, "core.dashboard", id)
		return err == nil
	}, "must be the id of a dashboard you may read, or none")
	if raw, given := input["defaults"]; given {
		m, ok := raw.(map[string]any)
		if !ok {
			bad("defaults", "type", `must be an object of types, like {"probe.probe": {"kind": "fault"}}`)
		}
		for typeName, v := range m {
			if v == nil {
				writes = append(writes, write{"defaults." + typeName, nil})
				continue
			}
			fields, ok := v.(map[string]any)
			t, err := s.rec.Type(ctx, typeName)
			if err != nil || !ok {
				bad("defaults", "invalid", fmt.Sprintf("%s: an installed type with an object of field values", typeName))
				continue
			}
			for name := range fields {
				if f, ok := t.Field(name); !ok || f.Calculated() || f.Type == meta.FieldText && f.Format == meta.FormatRich {
					bad("defaults", "field", fmt.Sprintf("%s has no field %q to preset", typeName, name))
				}
			}
			writes = append(writes, write{"defaults." + typeName, fields})
		}
	}
	if len(ps) > 0 {
		return &records.ValidationError{Problems: ps}
	}
	for _, w := range writes {
		if w.key == "language" && personID != nil {
			if err := s.setLanguage(ctx, *personID, w.value); err != nil {
				return err
			}
			continue
		}
		if w.value == nil {
			if err := s.tq(ctx, account).DeleteAccountSetting(ctx, queries.DeleteAccountSettingParams{AccountID: account, Key: w.key}); err != nil {
				return fmt.Errorf("prefs: remove %s: %w", w.key, err)
			}
			continue
		}
		raw, _ := json.Marshal(w.value)
		if err := s.tq(ctx, account).UpsertAccountSetting(ctx, queries.UpsertAccountSettingParams{AccountID: account, Key: w.key, Value: raw}); err != nil {
			return fmt.Errorf("prefs: set %s: %w", w.key, err)
		}
	}
	return nil
}

// setLanguage writes the language at the person: one's own setting, whatever the rights on
// core.person say. A change in between is taken as it is (one retry).
func (s *Service) setLanguage(ctx context.Context, personID uuid.UUID, value any) error {
	sys := system(ctx)
	for range 2 {
		p, err := s.rec.Get(sys, "core.person", personID)
		if err != nil {
			return err
		}
		_, err = s.rec.Update(sys, "core.person", personID, p["version"].(int64), map[string]any{"language": value}, nil)
		if !errors.Is(err, records.ErrConflict) {
			return err
		}
	}
	return fmt.Errorf("%w: the person changed twice meanwhile", records.ErrConflict)
}

// keyTenant holds the tenant an account of the installation worked in last (PL44).
const keyTenant = "tenant"

// LastTenant is the tenant the account chose last, or "".
func (s *Service) LastTenant(ctx context.Context, account uuid.UUID) string {
	rows, err := s.tq(ctx, account).AccountSettings(ctx, account)
	if err != nil {
		return ""
	}
	for _, r := range rows {
		var v string
		if r.Key == keyTenant && json.Unmarshal(r.Value, &v) == nil {
			return v
		}
	}
	return ""
}

// RememberTenant keeps the tenant the account switched to, for its next session.
func (s *Service) RememberTenant(ctx context.Context, account uuid.UUID, tenant string) error {
	raw, _ := json.Marshal(tenant)
	return s.tq(ctx, account).UpsertAccountSetting(ctx, queries.UpsertAccountSettingParams{AccountID: account, Key: keyTenant, Value: raw})
}
