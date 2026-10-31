// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package webhooks sends events to subscribed URLs (Hauptplan E39, KF13, E100; PL25;
// GLPI Webhook, Inventar G165). A subscription belongs to an account (a person or an API
// client) and receives the events that account may read, with the payload of the event
// stream. The dispatcher of the outbox hands every event to Handle, which queues one
// delivery job per matching subscription; the job posts the event signed after Standard
// Webhooks (standardwebhooks.com) and River repeats it with a growing delay, eight attempts
// in about eight hours. Targets are https; plain http only for the hosts of the setting
// webhooks.allowed_http_hosts.
package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/events"
	"group24.de/itsm/internal/jobs"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/records"
)

// SettingAllowedHTTPHosts lists the hosts (root node) that may receive webhooks over plain
// http, e.g. ["receiver.intern", "127.0.0.1"].
const SettingAllowedHTTPHosts = "webhooks.allowed_http_hosts"

// MaxAttempts is how often a delivery is tried.
const MaxAttempts = 8

// backoff is the wait after attempt n (index n-1): 1, 5, 15, 30 minutes, then 1, 2, 4 hours;
// the eighth attempt comes about 7 h 50 min after the first.
var backoff = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour, 4 * time.Hour}

// Errors the HTTP layer maps.
var (
	ErrNotFound = errors.New("webhooks: not found")
	ErrInvalid  = errors.New("webhooks: invalid")
)

// Webhook is a subscription as the API shows it. Secret is set only right after it was
// created or rotated.
type Webhook struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	URL      string    `json:"url"`
	Owner    uuid.UUID `json:"owner"`
	Kinds    []string  `json:"kinds"`
	Types    []string  `json:"types"`
	Enabled  bool      `json:"enabled"`
	Position int64     `json:"position"`
	// Tenant is the tenant the subscription belongs to; it gets the events of that tenant only
	// (PL45). The root while the installation has no tenants.
	Tenant    string     `json:"tenant"`
	Secret    string     `json:"secret,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	CreatedBy *uuid.UUID `json:"created_by,omitempty"`
	UpdatedAt time.Time  `json:"updated_at"`
	UpdatedBy *uuid.UUID `json:"updated_by,omitempty"`
}

// Input creates or changes a subscription; nil leaves a value as it is (on create: empty).
type Input struct {
	Name    *string   `json:"name"`
	URL     *string   `json:"url"`
	Kinds   *[]string `json:"kinds"`
	Types   *[]string `json:"types"`
	Enabled *bool     `json:"enabled"`
}

// Delivery is one delivery job of a subscription.
type Delivery struct {
	Job         int64      `json:"job"`
	Position    int64      `json:"position"`
	State       string     `json:"state"`
	Attempt     int        `json:"attempt"`
	MaxAttempts int        `json:"max_attempts"`
	LastError   string     `json:"last_error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	ScheduledAt time.Time  `json:"scheduled_at"`
	FinalizedAt *time.Time `json:"finalized_at,omitempty"`
}

// Settings reads a setting of a node (org.Service.Setting).
type Settings func(ctx context.Context, path, key string) (json.RawMessage, string, error)

// Rights loads the access of an account (authz.Service.Load).
type Rights func(ctx context.Context, account uuid.UUID) (*authz.Access, error)

// Service keeps the subscriptions and delivers.
type Service struct {
	db       *db.Router
	rec      *records.Service
	jobs     *jobs.Service
	settings Settings
	rights   Rights
	logger   *slog.Logger
	// Client sends the deliveries; tests replace it.
	Client *http.Client
	// Backoff is the wait after an attempt; tests shorten it.
	Backoff func(attempt int) time.Duration
	// Queue takes the delivery jobs (bulk); tests use their own, so that a running instance on
	// the same database does not take theirs.
	Queue string
}

// New wires the service. settings may be nil (no http hosts allowed).
func New(rt *db.Router, rec *records.Service, js *jobs.Service, settings Settings, rights Rights, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{db: rt, rec: rec, jobs: js, settings: settings, rights: rights, logger: logger,
		Backoff: Backoff, Queue: jobs.QueueBulk,
		Client: &http.Client{
			Timeout: 10 * time.Second,
			// A redirect is no success (Standard Webhooks): the receiver must answer itself.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}}
}

// tq is the query handle in the database of the request's tenant (PL47).
func (s *Service) tq(ctx context.Context) *queries.Queries { return queries.New(s.db.Tenant(ctx)) }

// --- subscriptions ---

var filterRe = regexp.MustCompile(`^[a-z][a-z0-9_.]{0,126}$`)

// Create adds a subscription for owner in the tenant of the request, starting after the
// newest event, and returns it with its secret.
func (s *Service) Create(ctx context.Context, in Input, owner uuid.UUID, actor *uuid.UUID) (Webhook, error) {
	w := Webhook{Enabled: true, Kinds: []string{}, Types: []string{}}
	if err := s.apply(ctx, &w, in); err != nil {
		return Webhook{}, err
	}
	if w.Name == "" || w.URL == "" {
		return Webhook{}, fmt.Errorf("%w: name and url are required", ErrInvalid)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Webhook{}, err
	}
	secret, err := NewSecret()
	if err != nil {
		return Webhook{}, err
	}
	latest, err := s.tq(ctx).LatestEventPosition(ctx)
	if err != nil {
		return Webhook{}, fmt.Errorf("webhooks: latest event: %w", err)
	}
	row, err := s.tq(ctx).InsertWebhook(ctx, queries.InsertWebhookParams{ID: id, Name: w.Name, Url: w.URL, Secret: secret, Owner: owner,
		Kinds: w.Kinds, Types: w.Types, Enabled: w.Enabled, Position: latest, CreatedBy: nullUUID(actor), Tenant: authz.From(ctx).CurrentTenant()})
	if err != nil {
		return Webhook{}, fmt.Errorf("webhooks: create: %w", err)
	}
	out := toWebhook(row)
	out.Secret = secret
	return out, nil
}

// Get returns a subscription of the request's tenant without its secret; one of another
// tenant is not there (PL45).
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Webhook, error) {
	row, err := s.tq(ctx).GetWebhook(ctx, id)
	if err == nil && !authz.From(ctx).InTenant(row.Tenant) {
		err = pgx.ErrNoRows
	}
	if err != nil {
		return Webhook{}, notFound(id, err)
	}
	return toWebhook(row), nil
}

// List returns the subscriptions of the request's tenant without secrets.
func (s *Service) List(ctx context.Context) ([]Webhook, error) {
	rows, err := s.tq(ctx).ListWebhooks(ctx)
	if err != nil {
		return nil, fmt.Errorf("webhooks: list: %w", err)
	}
	a := authz.From(ctx)
	out := make([]Webhook, 0, len(rows))
	for _, r := range rows {
		if a.InTenant(r.Tenant) {
			out = append(out, toWebhook(r))
		}
	}
	return out, nil
}

// Update changes the given values of a subscription.
func (s *Service) Update(ctx context.Context, id uuid.UUID, in Input, actor *uuid.UUID) (Webhook, error) {
	w, err := s.Get(ctx, id)
	if err != nil {
		return Webhook{}, err
	}
	if err := s.apply(ctx, &w, in); err != nil {
		return Webhook{}, err
	}
	row, err := s.tq(ctx).UpdateWebhook(ctx, queries.UpdateWebhookParams{ID: id, Name: w.Name, Url: w.URL, Kinds: w.Kinds, Types: w.Types,
		Enabled: w.Enabled, UpdatedBy: nullUUID(actor)})
	if err != nil {
		return Webhook{}, notFound(id, err)
	}
	return toWebhook(row), nil
}

// Rotate gives a subscription a new secret and returns it; deliveries from now on use it.
func (s *Service) Rotate(ctx context.Context, id uuid.UUID, actor *uuid.UUID) (Webhook, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return Webhook{}, err
	}
	secret, err := NewSecret()
	if err != nil {
		return Webhook{}, err
	}
	row, err := s.tq(ctx).SetWebhookSecret(ctx, queries.SetWebhookSecretParams{ID: id, Secret: secret, UpdatedBy: nullUUID(actor)})
	if err != nil {
		return Webhook{}, notFound(id, err)
	}
	out := toWebhook(row)
	out.Secret = secret
	return out, nil
}

// Delete removes a subscription; its queued deliveries end without sending.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	n, err := s.tq(ctx).DeleteWebhook(ctx, id)
	if err != nil {
		return fmt.Errorf("webhooks: delete: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("%w: webhook %s", ErrNotFound, id)
	}
	// Its positions in the tenants' databases go with it (PL47d).
	if s.db.Separate() {
		for _, d := range s.db.Databases()[1:] {
			if err := queries.New(d.Pool()).DeleteWebhookPosition(ctx, id); err != nil {
				return fmt.Errorf("webhooks: delete position in %s: %w", d.Name, err)
			}
		}
	}
	return nil
}

// Deliveries lists the newest delivery jobs of a subscription (at most limit, default 50).
func (s *Service) Deliveries(ctx context.Context, id uuid.UUID, limit int) ([]Delivery, error) {
	w, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	// river_job belongs to River; the delivery jobs carry the subscription in their arguments.
	// They lie where the dispatcher queued them (PL47c): in the database of the subscription,
	// and for one of the installation with a database per tenant in every database (PL47d).
	pools := []*pgxpool.Pool{s.db.Tenant(ctx)}
	if s.db.Separate() && w.Tenant == org.Root {
		pools = s.db.Pools()
	}
	out := []Delivery{}
	for _, pool := range pools {
		rows, err := pool.Query(ctx, `SELECT id, state, attempt, max_attempts, (args->>'position')::bigint, created_at,
	       scheduled_at, finalized_at, coalesce(errors[array_length(errors, 1)]->>'error', '')
	FROM river_job WHERE kind = $1 AND args->>'webhook' = $2 ORDER BY id DESC LIMIT $3`,
			DeliverArgs{}.Kind(), id.String(), limit)
		if err != nil {
			return nil, fmt.Errorf("webhooks: deliveries: %w", err)
		}
		for rows.Next() {
			var d Delivery
			var finalized *time.Time
			if err := rows.Scan(&d.Job, &d.State, &d.Attempt, &d.MaxAttempts, &d.Position, &d.CreatedAt, &d.ScheduledAt, &finalized, &d.LastError); err != nil {
				rows.Close()
				return nil, err
			}
			d.FinalizedAt = finalized
			out = append(out, d)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	if len(pools) > 1 {
		slices.SortStableFunc(out, func(a, b Delivery) int { return b.CreatedAt.Compare(a.CreatedAt) })
		out = out[:min(len(out), limit)]
	}
	return out, nil
}

// apply checks and sets the given values.
func (s *Service) apply(ctx context.Context, w *Webhook, in Input) error {
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" || len([]rune(name)) > 200 {
			return fmt.Errorf("%w: name must have 1 to 200 characters", ErrInvalid)
		}
		w.Name = name
	}
	if in.URL != nil {
		if err := s.checkURL(ctx, *in.URL); err != nil {
			return err
		}
		w.URL = *in.URL
	}
	if in.Kinds != nil {
		for _, k := range *in.Kinds {
			if !filterRe.MatchString(k) {
				return fmt.Errorf("%w: kind %q", ErrInvalid, k)
			}
		}
		w.Kinds = append([]string{}, *in.Kinds...)
	}
	if in.Types != nil {
		for _, t := range *in.Types {
			if _, err := s.rec.Type(ctx, t); err != nil {
				return fmt.Errorf("%w: no type %q", ErrInvalid, t)
			}
		}
		w.Types = append([]string{}, *in.Types...)
	}
	if in.Enabled != nil {
		w.Enabled = *in.Enabled
	}
	return nil
}

// checkURL allows https, and http for the hosts of webhooks.allowed_http_hosts.
func (s *Service) checkURL(ctx context.Context, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("%w: url must be an absolute http(s) URL without user and fragment", ErrInvalid)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		for _, h := range s.allowedHTTPHosts(ctx) {
			if strings.EqualFold(h, u.Hostname()) {
				return nil
			}
		}
		return fmt.Errorf("%w: url must use https; http only for the hosts of the setting %s", ErrInvalid, SettingAllowedHTTPHosts)
	}
	return fmt.Errorf("%w: url must use https", ErrInvalid)
}

func (s *Service) allowedHTTPHosts(ctx context.Context) []string {
	if s.settings == nil {
		return nil
	}
	raw, _, err := s.settings(ctx, "root", SettingAllowedHTTPHosts)
	if err != nil || raw == nil {
		return nil
	}
	var hosts []string
	if json.Unmarshal(raw, &hosts) != nil {
		return nil
	}
	return hosts
}

// --- fan-out from the dispatcher ---

// HandleBatch is the consumer webhooks of the dispatcher (PL41): the enabled subscriptions are
// read once per batch and matched in memory; every event a subscription matches and its owner
// may read gets a delivery job, all jobs of the batch in one statement of the consumer's
// transaction, and every subscription's position moves to the end of the batch with them.
func (s *Service) HandleBatch(ctx context.Context, tx pgx.Tx, batch []records.Event) error {
	if len(batch) == 0 {
		return nil
	}
	q := queries.New(tx)
	subs, err := q.EnabledWebhooks(ctx)
	if err != nil {
		return fmt.Errorf("webhooks: subscriptions: %w", err)
	}
	last := batch[len(batch)-1].Position
	var params []river.InsertManyParams
	queue := func(w queries.Webhook, after int64, since time.Time) {
		for _, e := range batch {
			if e.Position <= after || !matches(w, e) || (!since.IsZero() && !eventAfter(e, since)) || !s.visible(ctx, w, e) {
				continue
			}
			params = append(params, river.InsertManyParams{Args: DeliverArgs{Webhook: w.ID, Position: e.Position, Tenant: w.Tenant},
				InsertOpts: &river.InsertOpts{Queue: s.Queue, MaxAttempts: MaxAttempts}})
		}
	}
	for _, w := range subs {
		if w.Position >= last {
			continue
		}
		queue(w, w.Position, time.Time{})
		if _, err := q.AdvanceWebhook(ctx, queries.AdvanceWebhookParams{ID: w.ID, Position: last}); err != nil {
			return fmt.Errorf("webhooks: advance %s: %w", w.ID, err)
		}
	}
	// With a database per tenant the subscriptions of the installation lie in the control
	// database; in a tenant's database they stand at a position of their own (PL47d). One
	// that has no position here yet starts with the events after it was made.
	if s.db.Separate() && s.db.Current(ctx).Name != "" {
		central, err := queries.New(s.db.Control()).EnabledWebhooks(ctx)
		if err != nil {
			return fmt.Errorf("webhooks: subscriptions of the installation: %w", err)
		}
		rows, err := q.WebhookPositions(ctx)
		if err != nil {
			return fmt.Errorf("webhooks: positions: %w", err)
		}
		at := make(map[uuid.UUID]int64, len(rows))
		for _, r := range rows {
			at[r.WebhookID] = r.Position
		}
		for _, w := range central {
			if w.Tenant != org.Root {
				continue
			}
			after, known := at[w.ID]
			if known && after >= last {
				continue
			}
			var since time.Time
			if !known {
				since = w.CreatedAt.Time
			}
			queue(w, after, since)
			if err := q.SetWebhookPosition(ctx, queries.SetWebhookPositionParams{WebhookID: w.ID, Position: last}); err != nil {
				return fmt.Errorf("webhooks: advance %s: %w", w.ID, err)
			}
		}
	}
	return s.jobs.EnqueueManyTx(ctx, tx, params)
}

// eventAfter reports whether an event happened after a moment (its time is RFC 3339).
func eventAfter(e records.Event, since time.Time) bool {
	at, err := time.Parse(time.RFC3339Nano, e.At)
	return err == nil && at.After(since)
}

// Handle runs one event as a batch of its own, in a transaction of its own (tests).
func (s *Service) Handle(ctx context.Context, e records.Event) error {
	return pgx.BeginFunc(ctx, s.db.Tenant(ctx), func(tx pgx.Tx) error {
		return s.HandleBatch(ctx, tx, []records.Event{e})
	})
}

// matches applies the filters of a subscription: empty kinds or types mean all.
func matches(w queries.Webhook, e records.Event) bool {
	return (len(w.Kinds) == 0 || slices.Contains(w.Kinds, e.Kind)) && (len(w.Types) == 0 || slices.Contains(w.Types, e.TypeName))
}

// visible tells whether the owner of a subscription may read the event, working in the tenant
// of the subscription (PL45).
func (s *Service) visible(ctx context.Context, w queries.Webhook, e records.Event) bool {
	if s.rights == nil {
		return false
	}
	a, err := s.rights(ctx, w.Owner)
	if err != nil || a == nil {
		return false // nil would be the system access; an owner without access sees nothing
	}
	// An owner who may no longer work in the tenant gets nothing.
	if a = a.Enter(w.Tenant); a.CurrentTenant() != w.Tenant {
		return false
	}
	return s.rec.VisibleEvent(authz.WithAccess(ctx, a), e)
}

// --- delivery ---

// DeliverArgs is one event for one subscription.
type DeliverArgs struct {
	Webhook  uuid.UUID `json:"webhook"`
	Position int64     `json:"position"`
	// Tenant is the tenant of the subscription, for the job log (PL45).
	Tenant string `json:"tenant,omitempty"`
}

// Kind names the job.
func (DeliverArgs) Kind() string { return "webhooks.deliver" }

type deliverWorker struct {
	river.WorkerDefaults[DeliverArgs]
	s *Service
}

// NextRetry follows the backoff of the plan instead of River's default.
func (w *deliverWorker) NextRetry(job *river.Job[DeliverArgs]) time.Time {
	return time.Now().Add(w.s.Backoff(job.Attempt))
}

// Work delivers; a delivery that fails on its last attempt, or that is given up, is kept in
// the failures of the dispatcher (PL41).
func (w *deliverWorker) Work(ctx context.Context, job *river.Job[DeliverArgs]) error {
	err := w.s.Deliver(ctx, job.Args)
	var cancel *rivertype.JobCancelError
	if err != nil && (job.Attempt >= job.MaxAttempts || errors.As(err, &cancel)) {
		pos, id := job.Args.Position, job.ID
		f := events.Failure{Kind: events.FailureWebhook, Target: job.Args.Webhook.String(), EventPosition: &pos, JobID: &id,
			Attempts: job.Attempt, Error: err.Error()}
		// The subscription, its events and failures are in the database of its tenant (PL47b).
		home := w.s.db.For(job.Args.Tenant)
		if ev, evErr := queries.New(home).GetEvent(ctx, pos); evErr == nil {
			f.TypeName, f.RecordID = ev.TypeName, &ev.RecordID
		}
		if ferr := events.RecordFailure(ctx, home, f); ferr != nil {
			w.s.logger.Error("webhooks: failure not kept", "error", ferr.Error())
		}
	}
	return err
}

// Backoff is the wait after the given attempt (from 1).
func Backoff(attempt int) time.Duration {
	i := min(max(attempt-1, 0), len(backoff)-1)
	return backoff[i]
}

// RegisterDeliveryJob adds the worker of webhooks.deliver.
func RegisterDeliveryJob(js *jobs.Service, s *Service) {
	jobs.AddWorker(js, &deliverWorker{s: s})
}

// Deliver sends one event to one subscription. A deleted or disabled subscription, an event
// gone by retention or no longer visible to the owner ends the job without sending; a URL
// that is no longer allowed cancels it; anything but a 2xx answer is an error River repeats.
func (s *Service) Deliver(ctx context.Context, a DeliverArgs) error {
	// The job runs without a session: the subscription names its tenant (PL47b); the event
	// lies in the database the job runs in, which for a subscription of the installation with
	// a database per tenant is a tenant's (PL47d).
	q := queries.New(s.db.For(a.Tenant))
	row, err := q.GetWebhook(ctx, a.Webhook)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if !row.Enabled {
		return nil
	}
	ev, err := queries.New(s.db.Tenant(ctx)).GetEvent(ctx, a.Position)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.logger.Warn("webhooks: event gone before delivery", "webhook", a.Webhook, "position", a.Position)
			return nil
		}
		return err
	}
	e := records.Event{Position: ev.Position, At: ev.At.Time.UTC().Format("2006-01-02T15:04:05.000000Z07:00"), Kind: ev.Kind,
		TypeName: ev.TypeName, RecordID: ev.RecordID, Payload: ev.Payload}
	if ev.Actor.Valid {
		actor := ev.Actor.UUID
		e.Actor = &actor
	}
	if !s.visible(ctx, row, e) {
		return nil
	}
	if err := s.checkURL(ctx, row.Url); err != nil {
		return river.JobCancel(err)
	}
	body, err := json.Marshal(e)
	if err != nil {
		return err
	}
	msgID := "evt_" + strconv.FormatInt(e.Position, 10)
	ts := time.Now().Unix()
	sig, err := Sign(row.Secret, msgID, ts, body)
	if err != nil {
		return river.JobCancel(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, row.Url, bytes.NewReader(body))
	if err != nil {
		return river.JobCancel(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "itsm-webhooks")
	req.Header.Set("webhook-id", msgID)
	req.Header.Set("webhook-timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("webhook-signature", sig)
	resp, err := s.Client.Do(req)
	if err != nil {
		return fmt.Errorf("webhooks: post to %s: %w", row.Url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("webhooks: %s answered %d: %s", row.Url, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	return nil
}

// --- signatures (Standard Webhooks) ---

const secretPrefix = "whsec_"

// NewSecret returns a random secret in the form of Standard Webhooks: whsec_ and 32 bytes in
// base64.
func NewSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return secretPrefix + base64.StdEncoding.EncodeToString(b), nil
}

// Sign returns the value of webhook-signature: "v1," and the base64 HMAC-SHA256 of
// "id.timestamp.body" with the decoded secret.
func Sign(secret, id string, ts int64, body []byte) (string, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, secretPrefix))
	if err != nil {
		return "", fmt.Errorf("webhooks: secret: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + strconv.FormatInt(ts, 10) + "."))
	mac.Write(body)
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil)), nil
}

// Verify checks a received webhook as a receiver would: the timestamp within tolerance of
// now and one of the space-separated signatures matching.
func Verify(secret string, h http.Header, body []byte, tolerance time.Duration) error {
	ts, err := strconv.ParseInt(h.Get("webhook-timestamp"), 10, 64)
	if err != nil {
		return errors.New("webhooks: no timestamp")
	}
	if d := time.Since(time.Unix(ts, 0)); d > tolerance || d < -tolerance {
		return errors.New("webhooks: timestamp outside the tolerance")
	}
	want, err := Sign(secret, h.Get("webhook-id"), ts, body)
	if err != nil {
		return err
	}
	for _, got := range strings.Fields(h.Get("webhook-signature")) {
		if hmac.Equal([]byte(got), []byte(want)) {
			return nil
		}
	}
	return errors.New("webhooks: signature does not match")
}

func toWebhook(r queries.Webhook) Webhook {
	w := Webhook{ID: r.ID, Name: r.Name, URL: r.Url, Owner: r.Owner, Kinds: r.Kinds, Types: r.Types, Enabled: r.Enabled,
		Position: r.Position, Tenant: r.Tenant, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
	if w.Kinds == nil {
		w.Kinds = []string{}
	}
	if w.Types == nil {
		w.Types = []string{}
	}
	if r.CreatedBy.Valid {
		u := r.CreatedBy.UUID
		w.CreatedBy = &u
	}
	if r.UpdatedBy.Valid {
		u := r.UpdatedBy.UUID
		w.UpdatedBy = &u
	}
	return w
}

func notFound(id uuid.UUID, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: webhook %s", ErrNotFound, id)
	}
	return fmt.Errorf("webhooks: %w", err)
}

func nullUUID(u *uuid.UUID) uuid.NullUUID {
	if u == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *u, Valid: true}
}
