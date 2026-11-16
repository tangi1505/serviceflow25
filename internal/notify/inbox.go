// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/cache"
	"group24.de/itsm/internal/jobs"
	"group24.de/itsm/internal/records"
)

// The inbox (WF19, WF-Plan E19): a notification that has the way inbox leaves an entry per
// person and event in the table inbox_entry of the database of the event, rendered in the
// language of the person like the mail. Only the person reads, marks and removes her
// entries; a job removes those older than the setting notify.inbox_days. A new entry is told
// to the open tabs of the person over Valkey (InboxChannel), which the event stream passes on.

// SettingInboxDays is how many days an entry stays (default 90), read at the root.
const SettingInboxDays = "notify.inbox_days"

// DefaultInboxDays is the retention without the setting.
const DefaultInboxDays = 90

// InboxChannel is the Valkey channel of the notices of new entries.
const InboxChannel = "inbox"

// inboxPageMax bounds one page of the inbox.
const inboxPageMax = 200

// ErrInvalid says a call to the inbox cannot be fulfilled as given; it answers as 422.
var ErrInvalid = errors.New("notify: invalid")

// Entry is one entry of the inbox.
type Entry struct {
	ID             uuid.UUID  `json:"id"`
	NotificationID *uuid.UUID `json:"notification_id,omitempty"`
	Kind           string     `json:"kind"`
	TypeName       string     `json:"type_name"`
	RecordID       uuid.UUID  `json:"record_id"`
	Actor          string     `json:"actor"`
	Subject        string     `json:"subject"`
	Body           string     `json:"body"`
	CreatedAt      time.Time  `json:"created_at"`
	ReadAt         *time.Time `json:"read_at,omitempty"`
}

// InboxPage is one page of the inbox, newest first, with the number of the unread.
type InboxPage struct {
	Items  []Entry `json:"items"`
	Next   string  `json:"next,omitempty"`
	Unread int     `json:"unread"`
}

// InboxNotice tells the open tabs of a person that her inbox changed; the id of a person is
// unique over every database.
type InboxNotice struct {
	Person uuid.UUID `json:"person"`
	Unread int       `json:"unread"`
}

// SetKV gives the service Valkey for the notices; without it the tabs learn of new entries on
// their next read.
func (s *Service) SetKV(kv *cache.Client) { s.kv = kv }

// sessionPerson is the person of the request; the system and accounts without a person have
// no inbox.
func sessionPerson(ctx context.Context) (uuid.UUID, error) {
	acc := authz.From(ctx)
	if acc == nil || acc.PersonID == nil {
		return uuid.Nil, authz.ErrNoPerson
	}
	return *acc.PersonID, nil
}

// deliver keeps one entry and tells the open tabs; an entry that is there already (a job run
// twice) is not kept again.
func (s *Service) deliver(ctx context.Context, e Entry, person uuid.UUID, position int64) error {
	tag, err := s.db.Tenant(ctx).Exec(ctx, `INSERT INTO inbox_entry
(id, person_id, notification_id, event_position, kind, type_name, record_id, actor, subject, body)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (person_id, event_position, notification_id) DO NOTHING`,
		uuid.New(), person, e.NotificationID, position, e.Kind, e.TypeName, e.RecordID, e.Actor, e.Subject, e.Body)
	if err != nil {
		return fmt.Errorf("notify: inbox: %w", err)
	}
	if tag.RowsAffected() > 0 {
		s.tell(ctx, person)
	}
	return nil
}

// tell publishes the number of unread entries of a person for her open tabs.
func (s *Service) tell(ctx context.Context, person uuid.UUID) {
	if s.kv == nil {
		return
	}
	n, err := s.unread(ctx, person)
	if err != nil {
		s.logger.Warn("notify: inbox not told", "error", err.Error())
		return
	}
	raw, _ := json.Marshal(InboxNotice{Person: person, Unread: n})
	if err := s.kv.Publish(ctx, InboxChannel, string(raw)); err != nil {
		s.logger.Debug("notify: inbox notice not published", "error", err.Error())
	}
}

func (s *Service) unread(ctx context.Context, person uuid.UUID) (int, error) {
	var n int
	if err := s.db.Tenant(ctx).QueryRow(ctx, `SELECT count(*) FROM inbox_entry WHERE person_id = $1 AND read_at IS NULL`, person).Scan(&n); err != nil {
		return 0, fmt.Errorf("notify: inbox: %w", err)
	}
	return n, nil
}

// Inbox is a page of the session's inbox, newest first; unreadOnly leaves out the read ones,
// after continues from the id of the last entry of the page before.
func (s *Service) Inbox(ctx context.Context, unreadOnly bool, limit int, after string) (InboxPage, error) {
	person, err := sessionPerson(ctx)
	if err != nil {
		return InboxPage{}, err
	}
	if limit <= 0 || limit > inboxPageMax {
		limit = 50
	}
	args := []any{person, unreadOnly, limit + 1}
	cursor := ""
	if after != "" {
		id, err := uuid.Parse(after)
		if err != nil {
			return InboxPage{}, fmt.Errorf("%w: after is no entry", ErrInvalid)
		}
		args = append(args, id)
		cursor = ` AND (created_at, id) < (SELECT created_at, id FROM inbox_entry WHERE id = $4 AND person_id = $1)`
	}
	rows, err := s.db.Tenant(ctx).Query(ctx, `SELECT id, notification_id, kind, type_name, record_id, actor, subject, body, created_at, read_at
FROM inbox_entry WHERE person_id = $1 AND (NOT $2 OR read_at IS NULL)`+cursor+`
ORDER BY created_at DESC, id DESC LIMIT $3`, args...)
	if err != nil {
		return InboxPage{}, fmt.Errorf("notify: inbox: %w", err)
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Entry, error) {
		var e Entry
		return e, row.Scan(&e.ID, &e.NotificationID, &e.Kind, &e.TypeName, &e.RecordID, &e.Actor, &e.Subject, &e.Body, &e.CreatedAt, &e.ReadAt)
	})
	if err != nil {
		return InboxPage{}, fmt.Errorf("notify: inbox: %w", err)
	}
	page := InboxPage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.Next = page.Items[limit-1].ID.String()
	}
	if page.Items == nil {
		page.Items = []Entry{}
	}
	if page.Unread, err = s.unread(ctx, person); err != nil {
		return InboxPage{}, err
	}
	return page, nil
}

// MarkRead marks entries of the session as read, all of them with all; it returns the number
// of the unread left. Entries of others are left alone.
func (s *Service) MarkRead(ctx context.Context, ids []uuid.UUID, all bool) (int, error) {
	person, err := sessionPerson(ctx)
	if err != nil {
		return 0, err
	}
	if !all && len(ids) == 0 {
		return 0, fmt.Errorf("%w: name the entries or all", ErrInvalid)
	}
	if _, err := s.db.Tenant(ctx).Exec(ctx, `UPDATE inbox_entry SET read_at = now()
WHERE person_id = $1 AND read_at IS NULL AND ($2 OR id = ANY($3::uuid[]))`, person, all, ids); err != nil {
		return 0, fmt.Errorf("notify: inbox: %w", err)
	}
	s.tell(ctx, person)
	return s.unread(ctx, person)
}

// RemoveEntry removes one entry of the session for good; another's is ErrNotFound.
func (s *Service) RemoveEntry(ctx context.Context, id uuid.UUID) error {
	person, err := sessionPerson(ctx)
	if err != nil {
		return err
	}
	tag, err := s.db.Tenant(ctx).Exec(ctx, `DELETE FROM inbox_entry WHERE id = $1 AND person_id = $2`, id, person)
	if err != nil {
		return fmt.Errorf("notify: inbox: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return records.ErrNotFound
	}
	s.tell(ctx, person)
	return nil
}

// SubscribeInbox calls fn for every notice of a new or changed inbox until the context ends;
// without Valkey it waits for the end. The event stream runs it for the session's person.
func (s *Service) SubscribeInbox(ctx context.Context, fn func(InboxNotice)) error {
	if s.kv == nil {
		<-ctx.Done()
		return nil
	}
	return s.kv.Subscribe(ctx, InboxChannel, func(message string) {
		var n InboxNotice
		if json.Unmarshal([]byte(message), &n) == nil {
			fn(n)
		}
	})
}

// --- retention ---

// InboxRetentionArgs is the nightly job that removes old entries.
type InboxRetentionArgs struct{}

// Kind names the job and its schedule.
func (InboxRetentionArgs) Kind() string { return "notify.inbox_retention" }

type inboxRetentionWorker struct {
	river.WorkerDefaults[InboxRetentionArgs]
	s *Service
}

// Work removes the entries older than the retention of the job's database.
func (w *inboxRetentionWorker) Work(ctx context.Context, _ *river.Job[InboxRetentionArgs]) error {
	n, err := w.s.ExpireInbox(ctx, time.Now())
	if err != nil {
		return err
	}
	return river.RecordOutput(ctx, map[string]any{"removed": n})
}

// RegisterInboxRetention adds the nightly job and its worker; it runs in each database.
func (s *Service) RegisterInboxRetention(js *jobs.Service) error {
	jobs.AddWorker(js, &inboxRetentionWorker{s: s})
	return js.AddPeriodic(jobs.Periodic{
		Name:     InboxRetentionArgs{}.Kind(),
		Label:    map[string]string{"de": "Alte Einträge des Posteingangs entfernen", "en": "Expire old inbox entries"},
		Schedule: "45 2 * * *",
		Queue:    jobs.QueueBulk,
		Args:     func() river.JobArgs { return InboxRetentionArgs{} },
	})
}

// ExpireInbox removes the entries created before now minus the retention, read or not, and
// returns how many.
func (s *Service) ExpireInbox(ctx context.Context, now time.Time) (int64, error) {
	days := s.inboxDays(ctx)
	tag, err := s.db.Tenant(ctx).Exec(ctx, `DELETE FROM inbox_entry WHERE created_at < $1`, now.AddDate(0, 0, -days))
	if err != nil {
		return 0, fmt.Errorf("notify: inbox retention: %w", err)
	}
	if n := tag.RowsAffected(); n > 0 {
		s.logger.Info("inbox entries expired", "removed", n, "older_than_days", days)
	}
	return tag.RowsAffected(), nil
}

// inboxDays is the setting notify.inbox_days at the root, else the default.
func (s *Service) inboxDays(ctx context.Context) int {
	if s.Settings == nil {
		return DefaultInboxDays
	}
	raw, _, err := s.Settings(ctx, "root", SettingInboxDays)
	if err != nil || len(raw) == 0 {
		return DefaultInboxDays
	}
	var n float64
	if json.Unmarshal(raw, &n) == nil && n >= 1 {
		return int(n)
	}
	return DefaultInboxDays
}
