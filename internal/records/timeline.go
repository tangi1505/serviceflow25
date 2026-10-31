// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/richtext"
)

// --- timeline (PL42; Hauptplan E121; Inventar G40) ---
//
// A type with the trait timeline has entries in the core table entry: a text by its author,
// public or private. An entry never changes; a correction is a new entry. Writing one needs
// the right comment on the record, a private one also read_private; reading private entries
// needs read_private. An entry leaves the event comment but keeps the record's version, so
// that nobody with the record open gets a conflict. The timeline shows the entries together
// with the history of the record (field changes, transitions, breaches, escalations, notify
// of rules) in order of time.

// Rights of the timeline (special rights, any type may grant them).
const (
	RightComment     = "comment"
	RightReadPrivate = "read_private"
)

// EventComment is the event of a new entry; its payload names the entry and whether it is
// private, never its text.
const EventComment = "comment"

// MaxEntryLength is the longest text of an entry, in characters.
const MaxEntryLength = 100000

// Entry is one entry of a timeline.
type Entry struct {
	ID            uuid.UUID  `json:"id"`
	Body          string     `json:"body"`
	Format        string     `json:"format"`
	Private       bool       `json:"private"`
	CreatedAt     time.Time  `json:"created_at"`
	CreatedBy     *uuid.UUID `json:"created_by,omitempty"`
	CreatedByName *string    `json:"created_by_name,omitempty"`
}

// TimelineChange is one audit row as the timeline shows it (the AuditEntry of the history).
type TimelineChange struct {
	ID      uuid.UUID  `json:"id"`
	Version int64      `json:"version"`
	Action  string     `json:"action"`
	Field   *string    `json:"field,omitempty"`
	Old     any        `json:"old,omitempty"`
	New     any        `json:"new,omitempty"`
	Actor   *uuid.UUID `json:"actor,omitempty"`
	Rule    *string    `json:"rule,omitempty"`
	At      time.Time  `json:"at"`
}

// TimelineItem is an entry or a change, with the time, the account and its name.
type TimelineItem struct {
	Kind      string          `json:"kind"` // entry or change
	At        time.Time       `json:"at"`
	Actor     *uuid.UUID      `json:"actor,omitempty"`
	ActorName *string         `json:"actor_name,omitempty"`
	Entry     *Entry          `json:"entry,omitempty"`
	Change    *TimelineChange `json:"change,omitempty"`
}

// timelineType is a type with the trait timeline.
func (s *Service) timelineType(ctx context.Context, typeName string) (meta.Type, error) {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return meta.Type{}, err
	}
	if !t.HasTrait(meta.TraitTimeline) {
		return meta.Type{}, fmt.Errorf("%s has no timeline (trait timeline): %w", typeName, ErrNotFound)
	}
	return t, nil
}

// ReadsPrivate says whether the session may read the private entries of a record, and so
// the images of those entries (UI7).
func (s *Service) ReadsPrivate(ctx context.Context, typeName string, rec Record) bool {
	t, err := s.Type(ctx, typeName)
	return err == nil && readsPrivate(ctx, t, rec)
}

// readsPrivate says whether the session may read the private entries of the record.
func readsPrivate(ctx context.Context, t meta.Type, rec Record) bool {
	a := authz.From(ctx)
	return a.IsSystem() || a.Permits(t.QualifiedName(), RightReadPrivate, rec)
}

// Timeline lists the entries and the history of a record, oldest first. Private entries
// only with read_private; changes of fields the session may not read are left out.
func (s *Service) Timeline(ctx context.Context, typeName string, id uuid.UUID) ([]TimelineItem, error) {
	t, err := s.timelineType(ctx, typeName)
	if err != nil {
		return nil, err
	}
	rec, err := s.Get(ctx, typeName, id)
	if err != nil {
		return nil, err
	}
	q := queries.New(s.db.Tenant(ctx))
	rows, err := q.EntriesOfRecord(ctx, queries.EntriesOfRecordParams{TypeName: typeName, RecordID: id, WithPrivate: readsPrivate(ctx, t, rec)})
	if err != nil {
		return nil, fmt.Errorf("records: timeline: %w", err)
	}
	out := make([]TimelineItem, 0, len(rows))
	for _, r := range rows {
		e := entryFromRow(queries.GetEntryRow(r))
		out = append(out, TimelineItem{Kind: "entry", At: e.CreatedAt, Actor: e.CreatedBy, Entry: &e})
	}
	if t.HasHistory() {
		audits, err := q.ListAudit(ctx, queries.ListAuditParams{TypeName: typeName, RecordID: id})
		if err != nil {
			return nil, fmt.Errorf("records: timeline: %w", err)
		}
		readable := readableSet(ctx, t)
		for _, a := range audits {
			if a.Field.Valid && !readable(a.Field.String) {
				continue
			}
			c := changeFromAudit(a)
			out = append(out, TimelineItem{Kind: "change", At: c.At, Actor: c.Actor, Change: &c})
		}
	}
	// Both lists come ordered; ids are v7 and break ties within one moment.
	slices.SortStableFunc(out, func(a, b TimelineItem) int {
		if c := a.At.Compare(b.At); c != 0 {
			return c
		}
		return strings.Compare(a.itemID(), b.itemID())
	})
	if err := s.nameActors(ctx, q, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (it TimelineItem) itemID() string {
	if it.Entry != nil {
		return it.Entry.ID.String()
	}
	return it.Change.ID.String()
}

// nameActors fills in the display names of the accounts, with one query.
func (s *Service) nameActors(ctx context.Context, q *queries.Queries, items []TimelineItem) error {
	var ids []uuid.UUID
	for _, it := range items {
		if it.Actor != nil && !slices.Contains(ids, *it.Actor) {
			ids = append(ids, *it.Actor)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := q.AccountNames(ctx, ids)
	if err != nil {
		return fmt.Errorf("records: timeline names: %w", err)
	}
	names := make(map[uuid.UUID]string, len(rows))
	for _, r := range rows {
		names[r.ID] = r.DisplayName
	}
	for i := range items {
		if it := &items[i]; it.Actor != nil {
			if n, ok := names[*it.Actor]; ok {
				it.ActorName = &n
				if it.Entry != nil {
					it.Entry.CreatedByName = &n
				}
			}
		}
	}
	return nil
}

// AddEntry writes an entry to the record's timeline. The input has body, format (text or
// html, UI7) and private.
func (s *Service) AddEntry(ctx context.Context, typeName string, id uuid.UUID, input map[string]any, actor Actor) (Entry, error) {
	t, err := s.timelineType(ctx, typeName)
	if err != nil {
		return Entry{}, err
	}
	body, format, private, err := entryValues(input)
	if err != nil {
		return Entry{}, err
	}
	var out Entry
	err = s.tx(ctx, func(tx pgx.Tx) error {
		rec, err := s.read(ctx, tx, t, id, false, false)
		if err != nil {
			return err
		}
		if err := checkAccess(ctx, t, rec, RightComment); err != nil {
			return err
		}
		if private && !readsPrivate(ctx, t, rec) {
			return fmt.Errorf("%w: a private entry needs %s on %s", ErrForbidden, RightReadPrivate, t.QualifiedName())
		}
		out = Entry{Body: body, Format: format, Private: private}
		if out.ID, err = uuid.NewV7(); err != nil {
			return err
		}
		q := queries.New(tx)
		at, err := q.InsertEntry(ctx, queries.InsertEntryParams{ID: out.ID, TypeName: typeName, RecordID: id, Body: body, Format: format, Private: private, CreatedBy: nullActor(actor)})
		if err != nil {
			return fmt.Errorf("records: add entry: %w", err)
		}
		out.CreatedAt, out.CreatedBy = at.Time, actor
		version := rec["version"].(int64)
		return event(ctx, q, t, id, rec, EventComment, version, []string{},
			map[string]any{"entry": out.ID.String(), "private": private}, actor)
	})
	return out, s.written(ctx, typeName, err)
}

// GetEntry reads one entry of a record: as visible as the record, a private one only with
// read_private.
func (s *Service) GetEntry(ctx context.Context, typeName string, id, entryID uuid.UUID) (Entry, error) {
	t, err := s.timelineType(ctx, typeName)
	if err != nil {
		return Entry{}, err
	}
	rec, err := s.Get(ctx, typeName, id)
	if err != nil {
		return Entry{}, err
	}
	row, err := queries.New(s.db.Tenant(ctx)).GetEntry(ctx, queries.GetEntryParams{ID: entryID, TypeName: typeName, RecordID: id})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.Private && !readsPrivate(ctx, t, rec)) {
		return Entry{}, fmt.Errorf("entry %s of %s %s: %w", entryID, typeName, id, ErrNotFound)
	}
	if err != nil {
		return Entry{}, fmt.Errorf("records: entry: %w", err)
	}
	return entryFromRow(row), nil
}

// Formats of an entry (UI7): plain text, or HTML the server sanitized.
const (
	FormatText = "text"
	FormatHTML = "html"
)

// entryValues checks the input of a new entry; HTML comes back sanitized (UI7).
func entryValues(input map[string]any) (string, string, bool, error) {
	var ps []Problem
	for name := range input {
		if name != "body" && name != "private" && name != "format" {
			ps = append(ps, Problem{Field: name, Code: "unknown", Message: "is not a field of an entry; an entry has body, format and private"})
		}
	}
	format := FormatText
	if v, given := input["format"]; given && v != nil {
		switch f, _ := v.(string); f {
		case FormatText, FormatHTML:
			format = f
		default:
			ps = append(ps, Problem{Field: "format", Code: "choice", Message: "must be text or html"})
		}
	}
	body, ok := input["body"].(string)
	if ok && format == FormatHTML {
		body = richtext.Sanitize(body)
		if strings.TrimSpace(richtext.Text(body)) == "" && len(richtext.Images(body)) == 0 {
			// Nothing left but empty paragraphs: as empty as an empty text.
			body = ""
		}
	}
	switch {
	case input["body"] == nil || (ok && strings.TrimSpace(body) == ""):
		ps = append(ps, Problem{Field: "body", Code: "required", Message: "is required"})
	case !ok:
		ps = append(ps, Problem{Field: "body", Code: "type", Message: "must be text"})
	case len([]rune(body)) > MaxEntryLength:
		ps = append(ps, Problem{Field: "body", Code: "max_length", Message: fmt.Sprintf("is longer than %d characters", MaxEntryLength)})
	}
	private := false
	if v, given := input["private"]; given && v != nil {
		b, ok := v.(bool)
		if !ok {
			ps = append(ps, Problem{Field: "private", Code: "type", Message: "must be true or false"})
		}
		private = b
	}
	if len(ps) > 0 {
		return "", "", false, &ValidationError{Problems: ps}
	}
	return body, format, private, nil
}

func entryFromRow(r queries.GetEntryRow) Entry {
	e := Entry{ID: r.ID, Body: r.Body, Format: r.Format, Private: r.Private, CreatedAt: r.CreatedAt.Time}
	if r.CreatedBy.Valid {
		u := r.CreatedBy.UUID
		e.CreatedBy = &u
	}
	return e
}

func changeFromAudit(a queries.Audit) TimelineChange {
	c := TimelineChange{ID: a.ID, Version: a.Version, Action: a.Action, At: a.At.Time}
	if a.Field.Valid {
		f := a.Field.String
		c.Field = &f
		_ = json.Unmarshal(a.OldValue, &c.Old)
		_ = json.Unmarshal(a.NewValue, &c.New)
	}
	if a.Actor.Valid {
		u := a.Actor.UUID
		c.Actor = &u
	}
	if a.Rule.Valid {
		r := a.Rule.String
		c.Rule = &r
	}
	return c
}
