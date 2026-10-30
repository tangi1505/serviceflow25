// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package notify sends notifications as mail (WF15; Hauptplan KF14; Inventar G160, G161). A
// notification is a record of the core type core.notification: for the records of one type,
// on some kinds of events, if a CEL condition holds, to recipients (roles at the record,
// groups, persons), with a subject and a text that are translatable (PL21) and know a few
// placeholders. The notifications of rules (the action notify of WF4, WF10, WF11) go the same
// way with their message.
//
// The consumer notify of the dispatcher (PL41) turns each event into one job per matching
// notification; that job finds the recipients and queues one job per person, which renders
// the mail in the person's language and sends it over SMTP. A person gets one mail per event
// and notification, also when named twice, and none for what the person did. A mail that
// fails on its last attempt is kept with the failures.
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/events"
	"group24.de/itsm/internal/jobs"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/richtext"
	"group24.de/itsm/internal/rules"
)

// TypeNotification is the core type of the notifications.
const TypeNotification = "core.notification"

// Kinds are the events a notification can name; notify is the event of a rule's action and
// goes to the recipients the rule names. comment is a new entry of the timeline (PL42).
var Kinds = []string{"create", "update", "transition", "trash", "restore", "breach", "escalation", records.EventComment}

// MaxAttempts bounds the attempts of one mail.
const MaxAttempts = 8

// Rights loads what an account may do; the notifications use it for the person behind the
// account that caused an event.
type Rights func(ctx context.Context, account uuid.UUID) (*authz.Access, error)

// Service finds recipients and sends.
type Service struct {
	pool   *pgxpool.Pool
	q      *queries.Queries
	rec    *records.Service
	jobs   *jobs.Service
	rights Rights
	mail   *Mailer
	logger *slog.Logger
	// PublicURL is the address of the web interface, for {{link}}.
	PublicURL string
	// Queue is the River queue of the jobs; tests use their own.
	Queue string
}

// New wires the service.
func New(pool *pgxpool.Pool, rec *records.Service, js *jobs.Service, rights Rights, mail *Mailer, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{pool: pool, q: queries.New(pool), rec: rec, jobs: js, rights: rights, mail: mail, logger: logger, Queue: jobs.QueueDefault}
}

func system(ctx context.Context) context.Context {
	return authz.WithAccess(ctx, authz.System())
}

// --- the setting ---

// Check is the RecordCheck of records for core.notification: the type is installed, the
// events are known, every recipient is a role of the type, a group or a person, the condition
// compiles, and the placeholders of subject and text exist.
func (s *Service) Check(ctx context.Context, t meta.Type, rec map[string]any) []records.Problem {
	if t.QualifiedName() != TypeNotification {
		return nil
	}
	var ps []records.Problem
	add := func(field, code, msg string) {
		ps = append(ps, records.Problem{Field: field, Code: code, Message: msg})
	}
	name, _ := rec["target_type"].(string)
	target, err := s.rec.Type(ctx, name)
	if err != nil {
		add("target_type", "unknown", fmt.Sprintf("type %s is not installed", name))
		return ps
	}
	for _, k := range list(rec["events"]) {
		if !slices.Contains(Kinds, k) {
			add("events", "type", fmt.Sprintf("%q is no event; use %s", k, strings.Join(Kinds, ", ")))
		}
	}
	if len(list(rec["events"])) == 0 {
		add("events", "required", "a notification needs at least one event")
	}
	recipients := list(rec["recipients"])
	if len(recipients) == 0 {
		add("recipients", "required", "a notification needs recipients")
	}
	for _, r := range recipients {
		if msg := checkRecipient(target, r); msg != "" {
			add("recipients", "type", msg)
		}
	}
	if c, _ := rec["condition"].(string); c != "" {
		if _, err := rules.Compile(c); err != nil {
			add("condition", "type", err.Error())
		}
	}
	for _, field := range []string{"subject", "body"} {
		text, _ := rec[field].(string)
		if msg := checkPlaceholders(target, text); msg != "" {
			add(field, "type", msg)
		}
	}
	return ps
}

// list splits a list of names by commas, blanks or lines.
func list(v any) []string {
	s, _ := v.(string)
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == ';' })
}

// checkRecipient checks one recipient: actors.<role> of the type, or core.group:<id> or
// core.person:<id>.
func checkRecipient(t meta.Type, r string) string {
	if role, ok := strings.CutPrefix(r, "actors."); ok {
		if _, known := t.Actor(role); !known {
			return fmt.Sprintf("%q is not a role of %s", role, t.QualifiedName())
		}
		return ""
	}
	typ, id, ok := strings.Cut(r, ":")
	if _, err := uuid.Parse(id); !ok || err != nil || (typ != "core.group" && typ != "core.person") {
		return fmt.Sprintf("recipient %q must be actors.<role>, core.group:<id> or core.person:<id>", r)
	}
	return ""
}

// notification is a setting as the jobs use it.
type notification struct {
	ID uuid.UUID
	// OrgPath is the node of the notification: it acts on events of its tenant (PL45).
	OrgPath     string
	Name        string
	TypeName    string
	Events      []string
	Recipients  []string
	Condition   string
	NotifyActor bool
	Subject     string
	Body        string
}

func fromRecord(r records.Record) (notification, error) {
	id, err := uuid.Parse(fmt.Sprint(r["id"]))
	if err != nil {
		return notification{}, err
	}
	n := notification{ID: id, Name: fmt.Sprint(r["name"]), TypeName: fmt.Sprint(r["target_type"]), Events: list(r["events"]),
		Recipients: list(r["recipients"])}
	n.OrgPath, _ = r["org_path"].(string)
	n.Condition, _ = r["condition"].(string)
	n.NotifyActor, _ = r["notify_actor"].(bool)
	n.Subject, _ = r["subject"].(string)
	n.Body, _ = r["body"].(string)
	return n, nil
}

// active reads the active notifications; without the type (an instance not updated yet)
// there are none.
func (s *Service) active(ctx context.Context) ([]notification, error) {
	ctx = system(ctx)
	if _, err := s.rec.Type(ctx, TypeNotification); errors.Is(err, records.ErrNotFound) {
		return nil, nil
	}
	where, _ := json.Marshal(map[string]any{"field": "active", "eq": true})
	var out []notification
	after := ""
	for {
		page, _, err := s.rec.Query(ctx, TypeNotification, records.Query{Where: where, Limit: 500, After: after})
		if err != nil {
			return nil, fmt.Errorf("notify: settings: %w", err)
		}
		for _, r := range page.Items {
			n, err := fromRecord(r)
			if err != nil {
				return nil, err
			}
			out = append(out, n)
		}
		if page.Next == "" {
			return out, nil
		}
		after = page.Next
	}
}

// --- from events to jobs ---

// FanoutArgs is one event for one notification, or for a rule's notify (Notification nil).
type FanoutArgs struct {
	Position     int64      `json:"position"`
	Notification *uuid.UUID `json:"notification,omitempty"`
}

// Kind names the job.
func (FanoutArgs) Kind() string { return "notify.fanout" }

// SendArgs is one mail: an event, the notification (nil for a rule's notify) and a person.
type SendArgs struct {
	Position     int64      `json:"position"`
	Notification *uuid.UUID `json:"notification,omitempty"`
	Person       uuid.UUID  `json:"person"`
}

// Kind names the job.
func (SendArgs) Kind() string { return "notify.send" }

// HandleBatch is the consumer notify of the dispatcher (PL41): the active notifications are
// read once per batch; every event a notification names gets a job that finds its
// recipients, every notify of a rule one as well, all jobs in the consumer's transaction.
func (s *Service) HandleBatch(ctx context.Context, tx pgx.Tx, batch []records.Event) error {
	if len(batch) == 0 {
		return nil
	}
	notes, err := s.active(ctx)
	if err != nil {
		return err
	}
	tenants, err := org.TenantPaths(ctx, s.q)
	if err != nil {
		return err
	}
	var params []river.InsertManyParams
	opts := &river.InsertOpts{Queue: s.Queue}
	for _, e := range batch {
		if e.Kind == "notify" {
			params = append(params, river.InsertManyParams{Args: FanoutArgs{Position: e.Position}, InsertOpts: opts})
			continue
		}
		// A notification acts on the events of its own tenant only (PL45).
		tenant := org.TenantOf(tenants, records.EventPath(e))
		for _, n := range notes {
			if n.TypeName == e.TypeName && slices.Contains(n.Events, e.Kind) && org.TenantOf(tenants, n.OrgPath) == tenant {
				id := n.ID
				params = append(params, river.InsertManyParams{Args: FanoutArgs{Position: e.Position, Notification: &id}, InsertOpts: opts})
			}
		}
	}
	return s.jobs.EnqueueManyTx(ctx, tx, params)
}

// Register adds the workers of the jobs.
func (s *Service) Register(js *jobs.Service) {
	jobs.AddWorker(js, &fanoutWorker{s: s})
	jobs.AddWorker(js, &sendWorker{s: s})
}

type fanoutWorker struct {
	river.WorkerDefaults[FanoutArgs]
	s *Service
}

// Work finds the recipients of one event and queues one mail per person.
func (w *fanoutWorker) Work(ctx context.Context, job *river.Job[FanoutArgs]) error {
	persons, err := w.s.Recipients(ctx, job.Args)
	if err != nil || len(persons) == 0 {
		return err
	}
	params := make([]river.InsertManyParams, 0, len(persons))
	for _, p := range persons {
		params = append(params, river.InsertManyParams{Args: SendArgs{Position: job.Args.Position, Notification: job.Args.Notification, Person: p},
			// A repeated fan-out (the job ran, its end was not recorded) queues no mail twice.
			InsertOpts: &river.InsertOpts{Queue: w.s.Queue, MaxAttempts: MaxAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true}}})
	}
	if err := pgx.BeginFunc(ctx, w.s.pool, func(tx pgx.Tx) error { return w.s.jobs.EnqueueManyTx(ctx, tx, params) }); err != nil {
		return err
	}
	return river.RecordOutput(ctx, map[string]any{"persons": len(persons)})
}

// Recipients are the persons one event goes to for one notification, or for a rule's notify:
// roles at the record as it is now, groups by their members, persons; each once, without the
// person who caused the event unless the notification says so. A notification whose
// condition does not hold, an event gone by retention or a record gone have none.
func (s *Service) Recipients(ctx context.Context, a FanoutArgs) ([]uuid.UUID, error) {
	ctx = system(ctx)
	ev, err := s.q.GetEvent(ctx, a.Position)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rec, err := s.rec.Get(ctx, ev.TypeName, ev.RecordID)
	if errors.Is(err, records.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var targets []string
	notifyActor := false
	if a.Notification == nil {
		var payload struct {
			To []struct {
				Type string    `json:"type"`
				ID   uuid.UUID `json:"id"`
			} `json:"to"`
		}
		_ = json.Unmarshal(ev.Payload, &payload)
		for _, ref := range payload.To {
			targets = append(targets, ref.Type+":"+ref.ID.String())
		}
	} else {
		n, ok, err := s.notification(ctx, *a.Notification)
		if err != nil || !ok {
			return nil, err
		}
		if !s.holds(n, rec) {
			return nil, nil
		}
		targets, notifyActor = n.Recipients, n.NotifyActor
	}
	var out []uuid.UUID
	seen := map[uuid.UUID]bool{}
	addPerson := func(id uuid.UUID) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, target := range targets {
		var refs []string
		if role, ok := strings.CutPrefix(target, "actors."); ok {
			all, _ := rec["actors"].(map[string]any)
			holders, _ := all[role].([]any)
			for _, h := range holders {
				m, _ := h.(map[string]any)
				refs = append(refs, fmt.Sprint(m["type"])+":"+fmt.Sprint(m["id"]))
			}
		} else {
			refs = []string{target}
		}
		for _, ref := range refs {
			typ, raw, _ := strings.Cut(ref, ":")
			id, err := uuid.Parse(raw)
			if err != nil {
				continue
			}
			switch typ {
			case "core.person":
				addPerson(id)
			case "core.group":
				members, err := s.members(ctx, id)
				if err != nil {
					return nil, err
				}
				for _, m := range members {
					addPerson(m)
				}
			}
		}
	}
	// Persons of another tenant get nothing, whatever the notification or rule names (PL45).
	path, _ := rec["org_path"].(string)
	if out, err = s.ofTenant(ctx, out, path); err != nil {
		return nil, err
	}
	if !notifyActor && ev.Actor.Valid && s.rights != nil {
		if acc, err := s.rights(ctx, ev.Actor.UUID); err == nil && acc != nil && acc.PersonID != nil {
			out = slices.DeleteFunc(out, func(id uuid.UUID) bool { return id == *acc.PersonID })
		}
	}
	if ev.Kind == records.EventComment {
		// A private entry only goes to persons who may read it (PL42).
		var payload struct {
			Private bool `json:"private"`
		}
		_ = json.Unmarshal(ev.Payload, &payload)
		if payload.Private {
			allowed := out[:0]
			for _, p := range out {
				ok, err := s.readsPrivate(ctx, p, ev.TypeName, rec)
				if err != nil {
					return nil, err
				}
				if ok {
					allowed = append(allowed, p)
				}
			}
			out = allowed
		}
	}
	return out, nil
}

// ofTenant keeps the persons in the tenant of the node (PL45).
func (s *Service) ofTenant(ctx context.Context, persons []uuid.UUID, path string) ([]uuid.UUID, error) {
	tenants, err := org.TenantPaths(ctx, s.q)
	if err != nil || len(tenants) == 0 {
		return persons, err
	}
	tenant := org.TenantOf(tenants, path)
	kept := persons[:0]
	for _, id := range persons {
		p, err := s.rec.Get(ctx, "core.person", id)
		if errors.Is(err, records.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if at, _ := p["org_path"].(string); org.TenantOf(tenants, at) == tenant {
			kept = append(kept, id)
		}
	}
	return kept, nil
}

// readsPrivate says whether an active account of the person may read the private entries of
// the record (PL42); a person without an account may not.
func (s *Service) readsPrivate(ctx context.Context, person uuid.UUID, typeName string, rec records.Record) (bool, error) {
	if s.rights == nil {
		return false, nil
	}
	accounts, err := s.q.AccountsOfPerson(ctx, uuid.NullUUID{UUID: person, Valid: true})
	if err != nil {
		return false, err
	}
	for _, a := range accounts {
		if a.DisabledAt.Valid {
			continue
		}
		acc, err := s.rights(ctx, a.ID)
		if err != nil {
			return false, err
		}
		if acc != nil && acc.Permits(typeName, records.RightReadPrivate, rec) {
			return true, nil
		}
	}
	return false, nil
}

// holds evaluates the condition of a notification; one that cannot decide (a missing field)
// sends nothing and is logged.
func (s *Service) holds(n notification, rec records.Record) bool {
	if n.Condition == "" {
		return true
	}
	c, err := rules.Compile(n.Condition)
	if err == nil {
		var ok bool
		if ok, err = c.Eval(rec); err == nil {
			return ok
		}
	}
	s.logger.Warn("notify: condition failed", "notification", n.Name, "error", err.Error())
	return false
}

// notification reads one setting; a removed or inactive one is none.
func (s *Service) notification(ctx context.Context, id uuid.UUID) (notification, bool, error) {
	r, err := s.rec.Get(ctx, TypeNotification, id)
	if errors.Is(err, records.ErrNotFound) {
		return notification{}, false, nil
	}
	if err != nil {
		return notification{}, false, err
	}
	if active, _ := r["active"].(bool); !active {
		return notification{}, false, nil
	}
	// Subject and text in the language of the context (PL21), German without one.
	if _, err := s.rec.Localize(ctx, TypeNotification, r); err != nil {
		return notification{}, false, err
	}
	n, err := fromRecord(r)
	return n, err == nil, err
}

// members are the persons in a group.
func (s *Service) members(ctx context.Context, group uuid.UUID) ([]uuid.UUID, error) {
	var out []uuid.UUID
	after := ""
	for {
		page, err := s.rec.List(ctx, "core.group_member", records.ListOptions{Limit: 500, After: after, Filter: map[string]string{"group": group.String()}})
		if err != nil {
			return nil, err
		}
		for _, m := range page.Items {
			if id, err := uuid.Parse(fmt.Sprint(m["person"])); err == nil {
				out = append(out, id)
			}
		}
		if page.Next == "" {
			return out, nil
		}
		after = page.Next
	}
}

// --- sending ---

type sendWorker struct {
	river.WorkerDefaults[SendArgs]
	s *Service
}

// Work sends one mail; one that fails on its last attempt, or is given up, is kept with the
// failures of the dispatcher.
func (w *sendWorker) Work(ctx context.Context, job *river.Job[SendArgs]) error {
	to, err := w.s.Send(ctx, job.Args)
	var cancel *rivertype.JobCancelError
	if err != nil && (job.Attempt >= job.MaxAttempts || errors.As(err, &cancel)) {
		pos, id := job.Args.Position, job.ID
		target := "person " + job.Args.Person.String()
		if to != "" {
			target = to
		}
		f := events.Failure{Kind: events.FailureMail, Target: target, EventPosition: &pos, JobID: &id, Attempts: job.Attempt, Error: err.Error()}
		if ev, evErr := w.s.q.GetEvent(ctx, pos); evErr == nil {
			f.TypeName, f.RecordID = ev.TypeName, &ev.RecordID
		}
		if ferr := events.RecordFailure(ctx, w.s.pool, f); ferr != nil {
			w.s.logger.Error("notify: failure not kept", "error", ferr.Error())
		}
	}
	if err == nil && to != "" {
		return river.RecordOutput(ctx, map[string]any{"to": to})
	}
	return err
}

// Send renders one mail in the person's language and sends it; it returns the address. A
// person without an address, a record or event gone, or a notification switched off end the
// job without a mail.
func (s *Service) Send(ctx context.Context, a SendArgs) (string, error) {
	ctx = system(ctx)
	person, err := s.rec.Get(ctx, "core.person", a.Person)
	if errors.Is(err, records.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	to, _ := person["email"].(string)
	if strings.TrimSpace(to) == "" {
		s.logger.Info("notify: person without mail address", "person", a.Person)
		return "", nil
	}
	lang, _ := person["language"].(string)
	if lang == "" {
		lang = "de"
	}
	ctx = records.WithLang(ctx, records.Lang{Read: lang})
	ev, err := s.q.GetEvent(ctx, a.Position)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	t, err := s.rec.Type(ctx, ev.TypeName)
	if err != nil {
		return "", err
	}
	rec, err := s.rec.Get(ctx, ev.TypeName, ev.RecordID)
	if errors.Is(err, records.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if _, err := s.rec.Localize(ctx, ev.TypeName, rec); err != nil {
		return "", err
	}
	data := Data{Type: t, Record: rec, Lang: lang, Link: fmt.Sprintf("%s/types/%s/%s", s.PublicURL, ev.TypeName, ev.RecordID),
		Actor: s.actorName(ctx, ev.Actor)}
	if ev.Kind == records.EventComment {
		// The text of the new entry (PL42); an entry gone with its record sends nothing.
		var payload struct {
			Entry uuid.UUID `json:"entry"`
		}
		_ = json.Unmarshal(ev.Payload, &payload)
		e, err := s.rec.GetEntry(ctx, ev.TypeName, ev.RecordID, payload.Entry)
		if errors.Is(err, records.ErrNotFound) {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		data.Entry = e.Body
		if e.Format == records.FormatHTML {
			// Mails are plain text: the entry without its markup (UI7).
			data.Entry = richtext.Text(e.Body)
		}
	}
	subject, body := defaultSubject, defaultBody
	if a.Notification == nil {
		var payload struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(ev.Payload, &payload)
		data.Message = payload.Message
	} else {
		n, ok, err := s.notification(ctx, *a.Notification)
		if err != nil || !ok {
			return "", err
		}
		subject, body = n.Subject, n.Body
	}
	msg := Message{To: to, Subject: Render(subject, data), Body: Render(body, data), Date: time.Now()}
	if err := s.mail.Send(ctx, msg); err != nil {
		return to, fmt.Errorf("notify: mail to %s: %w", to, err)
	}
	return to, nil
}

// actorName is the name of the person behind the account that caused an event.
func (s *Service) actorName(ctx context.Context, account uuid.NullUUID) string {
	if !account.Valid || s.rights == nil {
		return "System"
	}
	acc, err := s.rights(ctx, account.UUID)
	if err != nil || acc == nil || acc.PersonID == nil {
		return "System"
	}
	p, err := s.rec.Get(ctx, "core.person", *acc.PersonID)
	if err != nil {
		return "System"
	}
	return strings.TrimSpace(fmt.Sprintf("%v %v", valueOr(p["first_name"]), valueOr(p["last_name"])))
}

func valueOr(v any) any {
	if v == nil {
		return ""
	}
	return v
}

// The mail of a rule's notify: its message and the link.
const (
	defaultSubject = "[{{type}}] {{title}}"
	defaultBody    = "{{message}}\n\n{{link}}"
)
