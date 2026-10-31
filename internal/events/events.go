// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package events is the outbox of the platform (Hauptplan E39, E46, E87, E112; architektur.md
// §7): the record service writes one event per change in the same transaction, the dispatcher
// in the worker hands them on in batches to its consumers (live for SSE over Valkey, webhooks,
// rules), each with its own position, and the stream lets a subscriber read from any position
// of the last days. No event is lost: a consumer that fails stays where it is and tries
// again, and the retention waits for it (PL41).
package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/cache"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/records"
)

// Channel is the Valkey channel the dispatcher publishes every event on, as JSON.
const Channel = "events"

// wakeChannel is the Valkey channel a write publishes on after its commit (Wake), so that
// the dispatcher reads at once instead of at its next poll.
const wakeChannel = "events.wake"

// batch is how many events the dispatcher and the stream read at once.
const batch = 1000

// ErrGone says the position is older than the retained events: the subscriber missed
// events and must resynchronise (410, architektur.md §7).
var ErrGone = errors.New("position too old")

// BatchHandler is a consumer of the dispatcher (live, webhooks, rules, …): it handles a batch
// of events in order, inside the consumer's transaction. What it writes in tx (jobs, its own
// positions) commits together with the consumer's new position, so an event leads to its
// jobs exactly once. An error rolls the pass back: the consumer stays where it was, the error
// is kept in event_consumer, and the next pass tries the same events again (PL41).
type BatchHandler func(ctx context.Context, tx pgx.Tx, batch []records.Event) error

// Consumers of the operation; a test gives its own Prefix.
const (
	ConsumerLive     = "live"
	ConsumerWebhooks = "webhooks"
	ConsumerRules    = "rules"
	// ConsumerNotify sends mail (WF15); it runs only where a mail server is set.
	ConsumerNotify = "notify"
)

type consumer struct {
	name string
	fn   BatchHandler
}

// Service reads the stream and runs the dispatcher.
type Service struct {
	db        *db.Router
	rec       *records.Service
	kv        *cache.Client
	logger    *slog.Logger
	consumers []consumer
	// Poll is the fallback interval of the dispatcher when no wake arrives.
	Poll time.Duration
	// Prefix goes before the consumers' names in event_consumer: empty in operation; tests on
	// a shared database use their own, which do not hold the retention back.
	Prefix string
	// StuckAfter is how long a consumer may fail before the log calls it stuck.
	StuckAfter time.Duration

	mu      sync.Mutex
	ensured map[string]bool
}

// New wires the service; kv may be nil in tests without Valkey (no Pub/Sub then). With
// Valkey the consumer "live" publishes every event for the SSE of every instance.
func New(rt *db.Router, rec *records.Service, kv *cache.Client, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	s := &Service{db: rt, rec: rec, kv: kv, logger: logger, Poll: time.Second, StuckAfter: 5 * time.Minute,
		ensured: map[string]bool{}}
	if kv != nil {
		s.Consume(ConsumerLive, s.publish)
	}
	return s
}

// Consume adds a consumer with its own position.
func (s *Service) Consume(name string, fn BatchHandler) {
	s.consumers = append(s.consumers, consumer{name: name, fn: fn})
}

// publish is the consumer live: one message per batch on the Valkey channel.
func (s *Service) publish(ctx context.Context, _ pgx.Tx, list []records.Event) error {
	raw, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return s.kv.Publish(ctx, Channel, string(raw))
}

// Latest is the position of the newest event (0 before the first).
func (s *Service) Latest(ctx context.Context) (int64, error) {
	n, err := queries.New(s.db.Tenant(ctx)).LatestEventPosition(ctx)
	if err != nil {
		return 0, fmt.Errorf("events: latest position: %w", err)
	}
	return n, nil
}

// Stream returns the events after a position that the session may see, oldest first. last
// is the position of the last event read (seen or not), which the caller continues from;
// more says that the batch was full. A position older than the retained events is ErrGone.
func (s *Service) Stream(ctx context.Context, after int64, limit int) (out []records.Event, last int64, more bool, err error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	oldest, err := queries.New(s.db.Tenant(ctx)).OldestEventPosition(ctx)
	if err != nil {
		return nil, 0, false, fmt.Errorf("events: oldest position: %w", err)
	}
	if oldest > 0 && after < oldest-1 {
		return nil, 0, false, fmt.Errorf("%w: after %d, the oldest kept event is %d", ErrGone, after, oldest)
	}
	raw, err := s.rec.Events(ctx, after, limit)
	if err != nil {
		return nil, 0, false, err
	}
	last = after
	out = make([]records.Event, 0, len(raw))
	for _, e := range raw {
		last = e.Position
		if s.rec.VisibleEvent(ctx, e) {
			out = append(out, e)
		}
	}
	return out, last, len(raw) == limit, nil
}

// Subscribe calls fn for every event the consumer live publishes until the context ends. It
// blocks; the SSE handler runs it in its own goroutine. Events of other instances arrive too.
// A message carries a batch of events (PL41).
func (s *Service) Subscribe(ctx context.Context, fn func(records.Event)) error {
	if s.kv == nil {
		<-ctx.Done()
		return nil
	}
	return s.kv.Subscribe(ctx, Channel, func(message string) {
		var list []records.Event
		if err := json.Unmarshal([]byte(message), &list); err != nil {
			return
		}
		for _, e := range list {
			fn(e)
		}
	})
}

// Wake tells the dispatcher (on any instance) that there is something new; the record
// service calls it after every commit (AfterWrite). It wakes the dispatcher of the database
// the write went to only (PL47c). Without Valkey it does nothing.
func (s *Service) Wake(ctx context.Context) {
	if s.kv == nil {
		return
	}
	if err := s.kv.Publish(ctx, wakeChannelOf(s.db.Current(ctx).Name), ""); err != nil {
		s.logger.Debug("events: wake not published", "error", err.Error())
	}
}

// wakeChannelOf is the wake channel of one database: the control database keeps the channel
// of one database, a tenant's database gets its name after it.
func wakeChannelOf(database string) string {
	if database == "" {
		return wakeChannel
	}
	return wakeChannel + "." + database
}

// RunDispatcher runs every consumer of every database in a loop of its own until the context
// ends (the worker role; PL47c: each database has its own stream and positions, read bound to
// it): a wake through Valkey starts a pass at once, a poll every Poll interval covers lost
// wakes, a full batch goes on at once, a failure waits from one second up to thirty.
func (s *Service) RunDispatcher(ctx context.Context) error {
	dbs := s.db.Databases()
	s.logger.Info("events: dispatcher started", "consumers", len(s.consumers), "databases", len(dbs))
	var wg sync.WaitGroup
	for _, d := range dbs {
		wg.Go(func() { s.dispatch(d.Bind(ctx), wakeChannelOf(d.Name)) })
	}
	wg.Wait()
	return nil
}

// dispatch runs the consumers of the database the context is bound to.
func (s *Service) dispatch(ctx context.Context, channel string) {
	wakes := make([]chan struct{}, len(s.consumers))
	for i := range wakes {
		wakes[i] = make(chan struct{}, 1)
	}
	if s.kv != nil {
		go func() {
			for ctx.Err() == nil {
				err := s.kv.Subscribe(ctx, channel, func(string) {
					for _, w := range wakes {
						select {
						case w <- struct{}{}:
						default:
						}
					}
				})
				if err != nil && ctx.Err() == nil {
					s.logger.Warn("events: wake subscription lost, retrying", "error", err.Error())
					select {
					case <-ctx.Done():
					case <-time.After(s.Poll):
					}
				}
			}
		}()
	}
	var wg sync.WaitGroup
	for i, c := range s.consumers {
		wg.Go(func() { s.loop(ctx, c, wakes[i]) })
	}
	wg.Wait()
}

func (s *Service) loop(ctx context.Context, c consumer, wake chan struct{}) {
	var backoff time.Duration
	for ctx.Err() == nil {
		n, err := s.pass(ctx, c)
		wait := s.Poll
		switch {
		case err != nil:
			backoff = min(max(2*backoff, time.Second), 30*time.Second)
			wait = backoff
		case n == batch:
			backoff = 0
			continue
		default:
			backoff = 0
		}
		if err != nil {
			select {
			case <-ctx.Done():
			case <-time.After(wait):
			}
			continue
		}
		select {
		case <-ctx.Done():
		case <-wake:
		case <-time.After(wait):
		}
	}
}

// Dispatch runs every consumer until it has caught up, once; a consumer that fails is left
// where it stands and the others go on. Tests call it directly.
func (s *Service) Dispatch(ctx context.Context) error {
	var errs []error
	for _, c := range s.consumers {
		for {
			n, err := s.pass(ctx, c)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", c.name, err))
				break
			}
			if n < batch {
				break
			}
		}
	}
	return errors.Join(errs...)
}

// pass is one batch of one consumer: lock its row (another instance working it means
// nothing to do here), read after its position, let it handle the batch, store the new
// position, all in one transaction. It returns how many events it handled.
func (s *Service) pass(ctx context.Context, c consumer) (int, error) {
	name := s.Prefix + c.name
	if err := s.ensure(ctx, name); err != nil {
		return 0, err
	}
	tx, err := s.db.Tenant(ctx).Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("events: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	position, err := queries.New(tx).LockConsumer(ctx, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("events: lock %s: %w", name, err)
	}
	list, err := s.rec.Events(ctx, position, batch)
	if err != nil {
		return 0, err
	}
	if len(list) == 0 {
		return 0, nil
	}
	if err := c.fn(ctx, tx, list); err != nil {
		_ = tx.Rollback(ctx)
		s.failed(ctx, name, list[0].Position, err)
		return 0, err
	}
	if err := queries.New(tx).ConsumerDone(ctx, queries.ConsumerDoneParams{Name: name, Position: list[len(list)-1].Position}); err != nil {
		return 0, fmt.Errorf("events: store position of %s: %w", name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("events: commit %s: %w", name, err)
	}
	return len(list), nil
}

// ensure makes the consumer's row on its first pass in this process: a new consumer starts at
// the end of the stream; a consumer of a test is not durable. Each database has its own row.
func (s *Service) ensure(ctx context.Context, name string) error {
	key := s.db.Current(ctx).Name + "/" + name
	s.mu.Lock()
	done := s.ensured[key]
	s.mu.Unlock()
	if done {
		return nil
	}
	latest, err := s.Latest(ctx)
	if err != nil {
		return err
	}
	if err := queries.New(s.db.Tenant(ctx)).EnsureConsumer(ctx, queries.EnsureConsumerParams{Name: name, Position: latest, Durable: s.Prefix == ""}); err != nil {
		return fmt.Errorf("events: consumer %s: %w", name, err)
	}
	s.mu.Lock()
	s.ensured[key] = true
	s.mu.Unlock()
	return nil
}

// failed keeps the error of a pass; a consumer failing for longer than StuckAfter is logged
// as stuck at every further failure.
func (s *Service) failed(ctx context.Context, name string, at int64, cause error) {
	row, err := queries.New(s.db.Tenant(ctx)).ConsumerFailed(ctx, queries.ConsumerFailedParams{Name: name, Error: pgtype.Text{String: cause.Error(), Valid: true}})
	if err != nil {
		s.logger.Error("events: consumer failed, state not kept", "consumer", name, "position", at, "error", cause.Error(), "store_error", err.Error())
		return
	}
	if row.FailingSince.Valid && time.Since(row.FailingSince.Time) > s.StuckAfter {
		s.logger.Error("events: consumer stuck", "consumer", name, "position", at, "since", row.FailingSince.Time, "attempts", row.Attempts, "error", cause.Error())
		return
	}
	s.logger.Warn("events: consumer failed, will retry", "consumer", name, "position", at, "attempts", row.Attempts, "error", cause.Error())
}

// SkipToEnd puts this service's consumers at the end of the stream; tests on a shared
// database start there.
func (s *Service) SkipToEnd(ctx context.Context) error {
	latest, err := s.Latest(ctx)
	if err != nil {
		return err
	}
	for _, c := range s.consumers {
		name := s.Prefix + c.name
		if err := s.ensure(ctx, name); err != nil {
			return err
		}
		if err := queries.New(s.db.Tenant(ctx)).ConsumerDone(ctx, queries.ConsumerDoneParams{Name: name, Position: latest}); err != nil {
			return err
		}
	}
	return nil
}

// ConsumerStatus is where a consumer stands and whether it fails (PL41).
type ConsumerStatus struct {
	Name         string     `json:"name"`
	Position     int64      `json:"position"`
	Lag          int64      `json:"lag"`
	Durable      bool       `json:"durable"`
	LastError    string     `json:"last_error,omitempty"`
	FailingSince *time.Time `json:"failing_since,omitempty"`
	Attempts     int        `json:"attempts"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// Consumers lists every consumer of the database with its lag behind the newest event.
func (s *Service) Consumers(ctx context.Context) ([]ConsumerStatus, error) {
	latest, err := s.Latest(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.New(s.db.Tenant(ctx)).ListConsumers(ctx)
	if err != nil {
		return nil, fmt.Errorf("events: consumers: %w", err)
	}
	out := make([]ConsumerStatus, 0, len(rows))
	for _, r := range rows {
		c := ConsumerStatus{Name: r.Name, Position: r.Position, Lag: max(latest-r.Position, 0), Durable: r.Durable,
			LastError: r.LastError.String, Attempts: int(r.Attempts), UpdatedAt: r.UpdatedAt.Time}
		if r.FailingSince.Valid {
			t := r.FailingSince.Time
			c.FailingSince = &t
		}
		out = append(out, c)
	}
	return out, nil
}

// Position is where a consumer of this service stands (0 before its first pass).
func (s *Service) Position(ctx context.Context, name string) int64 {
	n, err := queries.New(s.db.Tenant(ctx)).GetConsumerPosition(ctx, s.Prefix+name)
	if err != nil {
		return 0
	}
	return n
}

// --- what failed for good (PL41) ---

// Failure kinds.
const (
	FailureWebhook = "webhook"
	FailureRule    = "rule"
	FailureMail    = "mail"
)

// Failure is a webhook delivery or a rule that failed after all attempts of its job.
type Failure struct {
	ID            int64      `json:"id"`
	At            time.Time  `json:"at"`
	Kind          string     `json:"kind"`
	Target        string     `json:"target"`
	EventPosition *int64     `json:"event_position,omitempty"`
	TypeName      string     `json:"type_name,omitempty"`
	RecordID      *uuid.UUID `json:"record_id,omitempty"`
	JobID         *int64     `json:"job_id,omitempty"`
	Attempts      int        `json:"attempts"`
	Error         string     `json:"error"`
}

// RecordFailure keeps a failure; the workers of webhooks and rules call it on their last
// attempt or when they give up.
func RecordFailure(ctx context.Context, db queries.DBTX, f Failure) error {
	p := queries.InsertDeliveryFailureParams{Kind: f.Kind, Target: f.Target, Attempts: int32(f.Attempts), Error: f.Error} //nolint:gosec // attempts are few
	if f.EventPosition != nil {
		p.EventPosition = pgtype.Int8{Int64: *f.EventPosition, Valid: true}
	}
	if f.TypeName != "" {
		p.TypeName = pgtype.Text{String: f.TypeName, Valid: true}
	}
	if f.RecordID != nil {
		p.RecordID = uuid.NullUUID{UUID: *f.RecordID, Valid: true}
	}
	if f.JobID != nil {
		p.JobID = pgtype.Int8{Int64: *f.JobID, Valid: true}
	}
	if err := queries.New(db).InsertDeliveryFailure(ctx, p); err != nil {
		return fmt.Errorf("events: keep failure: %w", err)
	}
	return nil
}

// Failures lists the failures newest first, before an id (0: from the newest).
func (s *Service) Failures(ctx context.Context, before int64, limit int) ([]Failure, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if before <= 0 {
		before = math.MaxInt64
	}
	p := queries.ListDeliveryFailuresParams{Before: before, MaxRows: int32(limit)} //nolint:gosec // limit is at most 500
	if a := authz.From(ctx); a.Bounded() {
		p.Tenant = pgtype.Text{String: a.CurrentTenant(), Valid: true}
		p.Installation = a.Installation("core.job", authz.ActionRead)
	}
	rows, err := queries.New(s.db.Tenant(ctx)).ListDeliveryFailures(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("events: failures: %w", err)
	}
	out := make([]Failure, 0, len(rows))
	for _, r := range rows {
		f := Failure{ID: r.ID, At: r.At.Time, Kind: r.Kind, Target: r.Target, TypeName: r.TypeName.String, Attempts: int(r.Attempts), Error: r.Error}
		if r.EventPosition.Valid {
			n := r.EventPosition.Int64
			f.EventPosition = &n
		}
		if r.RecordID.Valid {
			id := r.RecordID.UUID
			f.RecordID = &id
		}
		if r.JobID.Valid {
			n := r.JobID.Int64
			f.JobID = &n
		}
		out = append(out, f)
	}
	return out, nil
}
