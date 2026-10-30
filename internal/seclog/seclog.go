// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package seclog is the security event log (Hauptplan E102; PL27; GLPI Event, Inventar
// G128): sign-ins on every way with success and failure, and changes of accounts, rights,
// login rules, API clients, directories and settings, each with time, account, source
// (address and user agent), outcome, reason and details. The log is append-only and readable
// with "read" on core.security_event; a nightly job removes entries older than the setting
// audit.security_retention (365 days by default).
package seclog

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/jobs"
)

// Categories of entries.
const (
	Auth   = "auth"   // sign-in, sign-out, token, context
	Change = "change" // accounts, rights, rules, clients, directories, settings
	Data   = "data"   // exports: data that leave the system (UI5)
)

// Outcomes.
const (
	Success = "success"
	Failure = "failure"
)

// Type is the type name the rights of the log are granted on.
const Type = "core.security_event"

// SettingRetention is the retention of the log (root node), in days.
const SettingRetention = "audit.security_retention"

// DefaultRetention is the retention without the setting.
const DefaultRetention = 365 * 24 * time.Hour

// Event is one entry as it is recorded.
type Event struct {
	Category  string
	Kind      string
	Outcome   string
	Account   *uuid.UUID
	Username  string
	IP        netip.Addr
	UserAgent string
	Reason    string
	Target    string
	Details   map[string]any
	// Tenant is the tenant the entry belongs to; empty for the installation (PL45).
	Tenant string
}

// Entry is one entry as the API shows it.
type Entry struct {
	ID        int64           `json:"id"`
	At        time.Time       `json:"at"`
	Category  string          `json:"category"`
	Kind      string          `json:"kind"`
	Outcome   string          `json:"outcome"`
	Account   *uuid.UUID      `json:"account,omitempty"`
	Username  string          `json:"username,omitempty"`
	IP        string          `json:"ip,omitempty"`
	UserAgent string          `json:"user_agent,omitempty"`
	Reason    string          `json:"reason,omitempty"`
	Target    string          `json:"target,omitempty"`
	Details   json.RawMessage `json:"details"`
	// Tenant is the tenant of the entry; absent for the installation (PL45).
	Tenant string `json:"tenant,omitempty"`
}

// Filter narrows a listing; zero values do not filter.
type Filter struct {
	Category string
	Kind     string
	Outcome  string
	Account  *uuid.UUID
	Username string
	IP       netip.Addr
	Since    time.Time
	Until    time.Time
	Before   int64 // the id of the last entry of the previous page
	Limit    int   // default 50, at most 500
	// Tenant keeps the entries of that tenant; Installation adds those of no tenant (PL45).
	Tenant       string
	Installation bool
}

// Page is a listing, newest first; Next is the cursor of the next page (0 on the last).
type Page struct {
	Items []Entry `json:"items"`
	Next  int64   `json:"next,omitempty"`
}

// Settings reads a setting of a node (org.Service.Setting).
type Settings func(ctx context.Context, path, key string) (json.RawMessage, string, error)

// Log writes and reads the security event log.
type Log struct {
	q        *queries.Queries
	settings Settings
	logger   *slog.Logger
}

// New returns the log; settings may be nil (default retention).
func New(pool *pgxpool.Pool, settings Settings, logger *slog.Logger) *Log {
	if logger == nil {
		logger = slog.Default()
	}
	return &Log{q: queries.New(pool), settings: settings, logger: logger}
}

// Record writes an entry. A failure to write is logged, not returned: the action it reports
// has happened already, and the log must not break sign-ins.
func (l *Log) Record(ctx context.Context, e Event) {
	if l == nil {
		return
	}
	details := []byte("{}")
	if len(e.Details) > 0 {
		if b, err := json.Marshal(e.Details); err == nil {
			details = b
		}
	}
	p := queries.InsertSecurityEventParams{Category: e.Category, Kind: e.Kind, Outcome: e.Outcome, Details: details,
		Username: text(e.Username), UserAgent: text(truncate(e.UserAgent, 500)), Reason: text(e.Reason), Target: text(e.Target)}
	if e.Account != nil {
		p.Account = uuid.NullUUID{UUID: *e.Account, Valid: true}
	}
	if e.IP.IsValid() {
		ip := e.IP
		p.Ip = &ip
	}
	p.Tenant = text(e.Tenant)
	// Without the request's cancellation: the entry belongs to an action that has happened.
	if err := l.q.InsertSecurityEvent(context.WithoutCancel(ctx), p); err != nil {
		l.logger.Error("security event not recorded", "kind", e.Kind, "outcome", e.Outcome, "error", err.Error())
	}
}

// List returns entries of one category, newest first.
func (l *Log) List(ctx context.Context, f Filter) (Page, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 500)
	p := queries.ListSecurityEventsParams{Category: f.Category, Kind: text(f.Kind), Outcome: text(f.Outcome),
		Username: text(f.Username), MaxRows: int32(limit + 1), //nolint:gosec // limit is at most 500
		Tenant: text(f.Tenant), Installation: f.Installation}
	if f.Before > 0 {
		p.Before = pgtype.Int8{Int64: f.Before, Valid: true}
	}
	if f.Account != nil {
		p.Account = uuid.NullUUID{UUID: *f.Account, Valid: true}
	}
	if f.IP.IsValid() {
		ip := f.IP
		p.Ip = &ip
	}
	if !f.Since.IsZero() {
		p.Since = pgtype.Timestamptz{Time: f.Since, Valid: true}
	}
	if !f.Until.IsZero() {
		p.Until = pgtype.Timestamptz{Time: f.Until, Valid: true}
	}
	rows, err := l.q.ListSecurityEvents(ctx, p)
	if err != nil {
		return Page{}, fmt.Errorf("seclog: list: %w", err)
	}
	page := Page{Items: make([]Entry, 0, len(rows))}
	for i, r := range rows {
		if i == limit {
			page.Next = page.Items[limit-1].ID
			break
		}
		e := Entry{ID: r.ID, At: r.At.Time.UTC(), Category: r.Category, Kind: r.Kind, Outcome: r.Outcome, IP: r.Ip,
			Username: r.Username.String, UserAgent: r.UserAgent.String, Reason: r.Reason.String, Target: r.Target.String, Details: r.Details,
			Tenant: r.Tenant}
		if r.Account.Valid {
			a := r.Account.UUID
			e.Account = &a
		}
		page.Items = append(page.Items, e)
	}
	return page, nil
}

// Retention is the setting audit.security_retention, or the default.
func (l *Log) Retention(ctx context.Context) time.Duration {
	if l.settings == nil {
		return DefaultRetention
	}
	raw, _, err := l.settings(ctx, "root", SettingRetention)
	if err != nil || raw == nil {
		return DefaultRetention
	}
	var days int
	if json.Unmarshal(raw, &days) != nil || days < 1 {
		return DefaultRetention
	}
	return time.Duration(days) * 24 * time.Hour
}

// Expire removes the entries older than the retention and returns how many.
func (l *Log) Expire(ctx context.Context) (int64, error) {
	keep := l.Retention(ctx)
	n, err := l.q.DeleteSecurityEventsBefore(ctx, pgtype.Timestamptz{Time: time.Now().Add(-keep), Valid: true})
	if err != nil {
		return 0, fmt.Errorf("seclog: expire: %w", err)
	}
	l.logger.Info("security events expired", "deleted", n, "older_than", keep.String())
	return n, nil
}

// RetentionArgs is the nightly job that applies the retention.
type RetentionArgs struct{}

// Kind names the job.
func (RetentionArgs) Kind() string { return "seclog.retention" }

type retentionWorker struct {
	river.WorkerDefaults[RetentionArgs]
	log *Log
}

func (w *retentionWorker) Work(ctx context.Context, _ *river.Job[RetentionArgs]) error {
	_, err := w.log.Expire(ctx)
	return err
}

// RegisterRetentionJob adds the worker and the nightly schedule.
func RegisterRetentionJob(js *jobs.Service, l *Log) error {
	jobs.AddWorker(js, &retentionWorker{log: l})
	return js.AddPeriodic(jobs.Periodic{
		Name:     RetentionArgs{}.Kind(),
		Label:    map[string]string{"de": "Ereignisprotokoll aufräumen", "en": "Expire the security event log"},
		Schedule: "45 2 * * *",
		Queue:    jobs.QueueDefault,
		Args:     func() river.JobArgs { return RetentionArgs{} },
	})
}

func text(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
